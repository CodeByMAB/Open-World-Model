// Package registry manages the lifecycle of OWM nodes: registration,
// heartbeat tracking, status transitions, and capability queries.
package registry

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/metrics"
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
	NodeID             uuid.UUID
	PublicKey          string   // Ed25519 hex
	LNNodeURI          string   // pubkey@host:port (clearnet or .onion)
	OnionAddress       string   // optional Tor v3 .onion hostname for control-plane access
	Tier               string
	VRAMGB             float64
	RAMGB              float64
	BandwidthMbps      float64
	SupportedTaskTypes []string // task types this node accepts; empty = all types
	Reliability        float64  // 0.0–1.0 rolling 7-day
	TotalTasks         int64
	TotalSats          int64
	Status             string
	RegisteredAt       time.Time
	LastHeartbeat      *time.Time
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
// request signature. onionAddr is optional (empty string if the node does not
// publish a Tor address). Returns the node ID and initial status.
func (r *Registry) Register(ctx context.Context, pubKeyHex, lnURI, onionAddr string, caps NodeCapabilities, sig []byte, timestamp int64) (*Node, error) {
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
	supportedTypes := caps.SupportedTaskTypes
	if supportedTypes == nil {
		supportedTypes = []string{}
	}
	node := &Node{
		NodeID:             uuid.New(),
		PublicKey:          pubKeyHex,
		LNNodeURI:          lnURI,
		OnionAddress:       onionAddr,
		Tier:               caps.Tier,
		VRAMGB:             caps.VRAMGB,
		RAMGB:              caps.RAMGB,
		BandwidthMbps:      caps.BandwidthMbps,
		SupportedTaskTypes: supportedTypes,
		Reliability:        1.0,
		Status:             StatusPending,
		RegisteredAt:       time.Now().UTC(),
	}

	// Capture existing tier/status before the upsert so re-registration can
	// decrement the prior label and keep metrics symmetric.
	const q = `
		WITH prior AS (
			SELECT tier, status FROM nodes WHERE public_key = $2
		)
		INSERT INTO nodes (node_id, public_key, ln_node_uri, onion_address, tier,
		                   vram_gb, ram_gb, bandwidth_mbps, reliability, status,
		                   registered_at, supported_task_types)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (public_key) DO UPDATE
			SET ln_node_uri          = EXCLUDED.ln_node_uri,
			    onion_address        = EXCLUDED.onion_address,
			    tier                 = EXCLUDED.tier,
			    vram_gb              = EXCLUDED.vram_gb,
			    ram_gb               = EXCLUDED.ram_gb,
			    bandwidth_mbps       = EXCLUDED.bandwidth_mbps,
			    supported_task_types = EXCLUDED.supported_task_types,
			    status               = 'pending',
			    registered_at        = EXCLUDED.registered_at
		RETURNING node_id, status,
		          (SELECT tier   FROM prior) AS prior_tier,
		          (SELECT status FROM prior) AS prior_status`

	row := r.db.QueryRow(ctx, q,
		node.NodeID, node.PublicKey, node.LNNodeURI, node.OnionAddress, node.Tier,
		node.VRAMGB, node.RAMGB, node.BandwidthMbps, node.Reliability,
		node.Status, node.RegisteredAt, node.SupportedTaskTypes,
	)

	var priorTier, priorStatus *string
	if err := row.Scan(&node.NodeID, &node.Status, &priorTier, &priorStatus); err != nil {
		return nil, fmt.Errorf("upserting node: %w", err)
	}

	// On re-registration the node had a prior status that must be decremented
	// before the new pending count is incremented.
	if priorStatus != nil {
		pt := node.Tier
		if priorTier != nil {
			pt = *priorTier
		}
		metrics.OwmNodesTotal.WithLabelValues(pt, *priorStatus).Dec()
	}
	metrics.OwmNodesTotal.WithLabelValues(node.Tier, node.Status).Inc()
	r.log.Info("node registered", zap.String("node_id", node.NodeID.String()), zap.String("tier", node.Tier))
	return node, nil
}

// Activate transitions a pending node to active after stake verification passes.
func (r *Registry) Activate(ctx context.Context, nodeID uuid.UUID) error {
	var tier string
	err := r.db.QueryRow(ctx,
		`UPDATE nodes SET status = $1 WHERE node_id = $2 AND status = $3 RETURNING tier`,
		StatusActive, nodeID, StatusPending,
	).Scan(&tier)
	if errors.Is(err, pgx.ErrNoRows) {
		// Node was not in pending state; nothing to do.
		return nil
	}
	if err != nil {
		return err
	}
	metrics.OwmNodesTotal.WithLabelValues(tier, StatusPending).Dec()
	metrics.OwmNodesTotal.WithLabelValues(tier, StatusActive).Inc()
	return nil
}

// RecordHeartbeat updates last_heartbeat, appends to heartbeat_log for uptime
// tracking (SRS-SCHED-04), and returns the number of pending tasks for that node.
func (r *Registry) RecordHeartbeat(ctx context.Context, nodeID uuid.UUID) (pendingTasks int, err error) {
	now := time.Now().UTC()
	err = r.db.QueryRow(ctx,
		`UPDATE nodes SET last_heartbeat = $1 WHERE node_id = $2
		 RETURNING (SELECT count(*) FROM tasks WHERE assigned_node = $2 AND status = 'pending')`,
		now, nodeID,
	).Scan(&pendingTasks)
	if err != nil {
		return 0, err
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO heartbeat_log (node_id, recorded_at) VALUES ($1, $2)`,
		nodeID, now,
	)
	return pendingTasks, err
}

// GetByID retrieves a node by its UUID.
func (r *Registry) GetByID(ctx context.Context, nodeID uuid.UUID) (*Node, error) {
	const q = `
		SELECT node_id, public_key, ln_node_uri, COALESCE(onion_address, ''), tier, vram_gb, ram_gb,
		       bandwidth_mbps, supported_task_types, reliability, total_tasks, total_sats,
		       status, registered_at, last_heartbeat
		FROM nodes WHERE node_id = $1`

	var n Node
	err := r.db.QueryRow(ctx, q, nodeID).Scan(
		&n.NodeID, &n.PublicKey, &n.LNNodeURI, &n.OnionAddress, &n.Tier,
		&n.VRAMGB, &n.RAMGB, &n.BandwidthMbps, &n.SupportedTaskTypes, &n.Reliability,
		&n.TotalTasks, &n.TotalSats, &n.Status, &n.RegisteredAt, &n.LastHeartbeat,
	)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	return &n, nil
}

// GetByPublicKey retrieves a node by its Ed25519 public key.
func (r *Registry) GetByPublicKey(ctx context.Context, pubKeyHex string) (*Node, error) {
	const q = `
		SELECT node_id, public_key, ln_node_uri, COALESCE(onion_address, ''), tier, vram_gb, ram_gb,
		       bandwidth_mbps, supported_task_types, reliability, total_tasks, total_sats,
		       status, registered_at, last_heartbeat
		FROM nodes WHERE public_key = $1`

	var n Node
	row := r.db.QueryRow(ctx, q, pubKeyHex)
	err := row.Scan(
		&n.NodeID, &n.PublicKey, &n.LNNodeURI, &n.OnionAddress, &n.Tier,
		&n.VRAMGB, &n.RAMGB, &n.BandwidthMbps, &n.SupportedTaskTypes, &n.Reliability,
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
		SELECT node_id, public_key, ln_node_uri, COALESCE(onion_address, ''), tier, vram_gb, ram_gb,
		       bandwidth_mbps, supported_task_types, reliability, total_tasks, total_sats,
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
			&n.NodeID, &n.PublicKey, &n.LNNodeURI, &n.OnionAddress, &n.Tier,
			&n.VRAMGB, &n.RAMGB, &n.BandwidthMbps, &n.SupportedTaskTypes, &n.Reliability,
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
	var tier, oldStatus string
	_ = r.db.QueryRow(ctx, `SELECT tier, status FROM nodes WHERE node_id = $1`, nodeID).Scan(&tier, &oldStatus)
	_, err := r.db.Exec(ctx,
		`UPDATE nodes SET status = $1 WHERE node_id = $2`,
		status, nodeID,
	)
	if err == nil && tier != "" {
		if oldStatus != "" {
			metrics.OwmNodesTotal.WithLabelValues(tier, oldStatus).Dec()
		}
		metrics.OwmNodesTotal.WithLabelValues(tier, status).Inc()
	}
	return err
}

// UpdateReliability recalculates and persists a node's reliability score using
// the full SRS-SCHED-04 formula over a rolling 7-day window:
//
//	reliability = task_success_fraction × uptime_fraction
//
// uptime_fraction is derived from heartbeat_log (SRS-NODE-04: 60 s interval).
// success=true increments the total_tasks counter; false does not (the task
// completion is already recorded in the tasks table and counted there).
func (r *Registry) UpdateReliability(ctx context.Context, nodeID uuid.UUID, success bool) error {
	// Clamp the window start to the node's registration time so brand-new nodes
	// are not penalised for the days before they existed.
	var registeredAt time.Time
	if err := r.db.QueryRow(ctx,
		`SELECT registered_at FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&registeredAt); err != nil {
		return fmt.Errorf("fetching registration time: %w", err)
	}
	windowStart := time.Now().UTC().Add(-7 * 24 * time.Hour)
	if registeredAt.After(windowStart) {
		windowStart = registeredAt
	}

	// Count tasks in the window.
	var totalTasks, completedTasks int64
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'completed')
		 FROM tasks WHERE assigned_node = $1 AND submitted_at > $2`,
		nodeID, windowStart,
	).Scan(&totalTasks, &completedTasks); err != nil {
		return fmt.Errorf("counting tasks: %w", err)
	}

	// Count heartbeats in the window.
	var receivedHB int64
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM heartbeat_log WHERE node_id = $1 AND recorded_at > $2`,
		nodeID, windowStart,
	).Scan(&receivedHB); err != nil {
		return fmt.Errorf("counting heartbeats: %w", err)
	}

	reliability := computeReliability(completedTasks, totalTasks, receivedHB, time.Since(windowStart))

	successIncr := 0
	if success {
		successIncr = 1
	}
	_, err := r.db.Exec(ctx,
		`UPDATE nodes SET reliability = $2, total_tasks = total_tasks + $3 WHERE node_id = $1`,
		nodeID, reliability, successIncr,
	)
	return err
}

// computeReliability implements SRS-SCHED-04:
//
//	reliability = task_success_fraction × uptime_fraction
//
// task_success_fraction = completed / max(total, 1)  [1.0 when no tasks yet]
// uptime_fraction       = receivedHB / expectedHB    [1.0 when window < 1 interval]
// expectedHB            = windowDuration / 60 s      (SRS-NODE-04 heartbeat rate)
//
// Both fractions are clamped to [0, 1] and the result is clamped to [0, 1].
func computeReliability(completed, total, receivedHB int64, windowDuration time.Duration) float64 {
	const heartbeatIntervalSecs = 60.0

	taskFraction := 1.0
	if total > 0 {
		taskFraction = float64(completed) / float64(total)
	}

	expectedHB := windowDuration.Seconds() / heartbeatIntervalSecs
	uptimeFraction := 1.0
	if expectedHB >= 1.0 {
		uptimeFraction = float64(receivedHB) / expectedHB
		if uptimeFraction > 1 {
			uptimeFraction = 1
		}
	}

	r := taskFraction * uptimeFraction
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
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
