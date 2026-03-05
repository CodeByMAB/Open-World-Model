// Package registry manages the lifecycle of OWM nodes: registration,
// heartbeat tracking, status transitions, and capability queries.
package registry

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Tier constants match the BRS-defined hardware tiers.
const (
	TierT1 = "t1"
	TierT2 = "t2"
	TierT3 = "t3"
)

// Status constants for node lifecycle.
const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusDegraded  = "degraded"
	StatusSuspended = "suspended"
)

// Node represents a registered OWM network participant.
type Node struct {
	NodeID        uuid.UUID
	PublicKey     string // Ed25519 hex
	LNNodeURI     string // pubkey@host:port
	Tier          string
	VRAMGB        float64
	RAMGB         float64
	BandwidthMbps float64
	Reliability   float64 // 0.0–1.0 rolling 7-day
	TotalTasks    int64
	TotalSats     int64
	Status        string
	RegisteredAt  time.Time
	LastHeartbeat *time.Time
}

// NodeCapabilities describes the hardware offered by a registering node.
type NodeCapabilities struct {
	Tier               string
	VRAMGB             float64
	RAMGB              float64
	BandwidthMbps      float64
	SupportedTaskTypes []string
}

// Registry provides node lifecycle operations backed by PostgreSQL.
type Registry struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

// New constructs a Registry with the given database pool.
func New(db *pgxpool.Pool, log *zap.Logger) *Registry {
	return &Registry{db: db, log: log}
}

// Register inserts a new node record in pending state after verifying the
// request signature. Returns the node ID and initial status.
func (r *Registry) Register(ctx context.Context, pubKeyHex, lnURI string, caps NodeCapabilities, sig []byte, timestamp int64) (*Node, error) {
	// Verify anti-replay: reject requests older than 5 minutes.
	reqTime := time.Unix(timestamp, 0)
	if time.Since(reqTime) > 5*time.Minute {
		return nil, fmt.Errorf("registration timestamp too old (anti-replay)")
	}

	// Verify Ed25519 signature over canonical message: pubkey|lnURI|tier|timestamp
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key")
	}
	msg := canonicalRegisterMessage(pubKeyHex, lnURI, caps.Tier, timestamp)
	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), msg, sig) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Upsert node — if pubkey already exists, update capabilities and reset to pending.
	node := &Node{
		NodeID:        uuid.New(),
		PublicKey:     pubKeyHex,
		LNNodeURI:     lnURI,
		Tier:          caps.Tier,
		VRAMGB:        caps.VRAMGB,
		RAMGB:         caps.RAMGB,
		BandwidthMbps: caps.BandwidthMbps,
		Reliability:   1.0,
		Status:        StatusPending,
		RegisteredAt:  time.Now().UTC(),
	}

	const q = `
		INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, vram_gb, ram_gb,
		                   bandwidth_mbps, reliability, status, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (public_key) DO UPDATE
			SET ln_node_uri    = EXCLUDED.ln_node_uri,
			    tier           = EXCLUDED.tier,
			    vram_gb        = EXCLUDED.vram_gb,
			    ram_gb         = EXCLUDED.ram_gb,
			    bandwidth_mbps = EXCLUDED.bandwidth_mbps,
			    status         = 'pending',
			    registered_at  = EXCLUDED.registered_at
		RETURNING node_id, status`

	row := r.db.QueryRow(ctx, q,
		node.NodeID, node.PublicKey, node.LNNodeURI, node.Tier,
		node.VRAMGB, node.RAMGB, node.BandwidthMbps, node.Reliability,
		node.Status, node.RegisteredAt,
	)
	if err := row.Scan(&node.NodeID, &node.Status); err != nil {
		return nil, fmt.Errorf("upserting node: %w", err)
	}

	r.log.Info("node registered", zap.String("node_id", node.NodeID.String()), zap.String("tier", node.Tier))
	return node, nil
}

// Activate transitions a pending node to active after stake verification passes.
func (r *Registry) Activate(ctx context.Context, nodeID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE nodes SET status = $1 WHERE node_id = $2 AND status = $3`,
		StatusActive, nodeID, StatusPending,
	)
	return err
}

// RecordHeartbeat updates last_heartbeat and node metrics, returning the
// number of pending tasks for that node.
func (r *Registry) RecordHeartbeat(ctx context.Context, nodeID uuid.UUID) (pendingTasks int, err error) {
	now := time.Now().UTC()
	err = r.db.QueryRow(ctx,
		`UPDATE nodes SET last_heartbeat = $1 WHERE node_id = $2
		 RETURNING (SELECT count(*) FROM tasks WHERE assigned_node = $2 AND status = 'pending')`,
		now, nodeID,
	).Scan(&pendingTasks)
	return pendingTasks, err
}

// GetByPublicKey retrieves a node by its Ed25519 public key.
func (r *Registry) GetByPublicKey(ctx context.Context, pubKeyHex string) (*Node, error) {
	const q = `
		SELECT node_id, public_key, ln_node_uri, tier, vram_gb, ram_gb,
		       bandwidth_mbps, reliability, total_tasks, total_sats,
		       status, registered_at, last_heartbeat
		FROM nodes WHERE public_key = $1`

	var n Node
	row := r.db.QueryRow(ctx, q, pubKeyHex)
	err := row.Scan(
		&n.NodeID, &n.PublicKey, &n.LNNodeURI, &n.Tier,
		&n.VRAMGB, &n.RAMGB, &n.BandwidthMbps, &n.Reliability,
		&n.TotalTasks, &n.TotalSats, &n.Status, &n.RegisteredAt, &n.LastHeartbeat,
	)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	return &n, nil
}

// ListActive returns all nodes currently in active status.
func (r *Registry) ListActive(ctx context.Context) ([]*Node, error) {
	const q = `
		SELECT node_id, public_key, ln_node_uri, tier, vram_gb, ram_gb,
		       bandwidth_mbps, reliability, total_tasks, total_sats,
		       status, registered_at, last_heartbeat
		FROM nodes WHERE status = 'active' ORDER BY reliability DESC`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.NodeID, &n.PublicKey, &n.LNNodeURI, &n.Tier,
			&n.VRAMGB, &n.RAMGB, &n.BandwidthMbps, &n.Reliability,
			&n.TotalTasks, &n.TotalSats, &n.Status, &n.RegisteredAt, &n.LastHeartbeat,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, &n)
	}
	return nodes, rows.Err()
}

// UpdateStatus sets a node's status directly. Used by the stake manager.
func (r *Registry) UpdateStatus(ctx context.Context, nodeID uuid.UUID, status string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE nodes SET status = $1 WHERE node_id = $2`,
		status, nodeID,
	)
	return err
}

// UpdateReliability recalculates and persists a node's reliability score.
// reliability = (successful_tasks / total_tasks) * uptime_fraction (rolling 7d).
func (r *Registry) UpdateReliability(ctx context.Context, nodeID uuid.UUID, success bool) error {
	var col string
	if success {
		col = "total_tasks = total_tasks + 1"
	} else {
		col = "total_tasks = total_tasks + 1"
	}
	// Simplified reliability update; a full implementation uses a time-windowed query.
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`UPDATE nodes SET %s, reliability = (
			SELECT COALESCE(
				COUNT(*) FILTER (WHERE status = 'completed')::NUMERIC /
				NULLIF(COUNT(*), 0), 1.0
			) FROM tasks WHERE assigned_node = $1
			  AND submitted_at > now() - INTERVAL '7 days'
		) WHERE node_id = $1`, col),
		nodeID,
	)
	return err
}

// IsSuspendedOrCoolingDown returns true if the node's public key is currently
// suspended and the slashing cooldown has not expired.
func (r *Registry) IsSuspendedOrCoolingDown(ctx context.Context, pubKeyHex string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM nodes n
		 JOIN slashing_events se ON se.node_id = n.node_id
		 WHERE n.public_key = $1 AND se.cooldown_expires_at > now()`,
		pubKeyHex,
	).Scan(&count)
	return count > 0, err
}

func canonicalRegisterMessage(pubKey, lnURI, tier string, ts int64) []byte {
	return []byte(fmt.Sprintf("owm-register|%s|%s|%s|%d", pubKey, lnURI, tier, ts))
}
