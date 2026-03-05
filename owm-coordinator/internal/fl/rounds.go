// Package fl orchestrates federated learning rounds across participating nodes
// (SRS-FL-01 through SRS-FL-07). Each round collects gradient updates, runs
// anomaly detection, performs FedAvg aggregation, and triggers OTS anchoring.
package fl

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/registry"
)

// RoundStatus enumerates federated learning round lifecycle states.
type RoundStatus string

const (
	RoundStatusOpen       RoundStatus = "open"
	RoundStatusAggregating RoundStatus = "aggregating"
	RoundStatusComplete   RoundStatus = "complete"
	RoundStatusFailed     RoundStatus = "failed"
)

// GradientSubmission represents a gradient delta from a participating node.
type GradientSubmission struct {
	NodeID       uuid.UUID
	RoundID      int32
	GradientData []byte
	GradientHash string // SHA-256 hex of gradient_data
}

// RoundSummary is returned after a round completes aggregation.
type RoundSummary struct {
	RoundID        int32
	ParticipantCount int
	AggregatedHash string // SHA-256 of the aggregated weights
	OTSPending     bool   // true until Bitcoin anchoring confirms
	CompletedAt    time.Time
}

// Orchestrator manages FL round lifecycle.
type Orchestrator struct {
	db       *pgxpool.Pool
	registry *registry.Registry
	log      *zap.Logger

	// minParticipants is the minimum number of gradient submissions required
	// before aggregation proceeds. Defaults to 3 (SRS-FL-03).
	minParticipants int
	// roundDurationSecs is how long a round stays open for submissions.
	roundDurationSecs int
}

// New creates an FL Orchestrator with sensible defaults.
func New(db *pgxpool.Pool, reg *registry.Registry, log *zap.Logger) *Orchestrator {
	return &Orchestrator{
		db:                db,
		registry:          reg,
		log:               log,
		minParticipants:   3,
		roundDurationSecs: 3600, // 1 hour
	}
}

// OpenRound creates a new FL round record and returns the round number.
// Only one round should be open at a time; callers must check before calling.
func (o *Orchestrator) OpenRound(ctx context.Context, modelVersionID string) (int32, error) {
	var roundNumber int32
	err := o.db.QueryRow(ctx,
		`INSERT INTO fl_rounds
		    (model_version_id, status, started_at, deadline_at)
		 VALUES ($1, 'open', now(), now() + ($2 || ' seconds')::INTERVAL)
		 RETURNING round_number`,
		modelVersionID,
		o.roundDurationSecs,
	).Scan(&roundNumber)
	if err != nil {
		return 0, fmt.Errorf("opening FL round: %w", err)
	}
	o.log.Info("FL round opened",
		zap.Int32("round_number", roundNumber),
		zap.String("model_version_id", modelVersionID),
	)
	return roundNumber, nil
}

// SubmitGradient records a gradient update from a node for the current round.
// Returns an error if the round is not open or the node is not eligible.
func (o *Orchestrator) SubmitGradient(ctx context.Context, sub GradientSubmission) error {
	// Verify the round is still open.
	var status RoundStatus
	var deadline time.Time
	err := o.db.QueryRow(ctx,
		`SELECT status, deadline_at FROM fl_rounds WHERE round_number = $1`,
		sub.RoundID,
	).Scan(&status, &deadline)
	if err != nil {
		return fmt.Errorf("fetching round %d: %w", sub.RoundID, err)
	}
	if status != RoundStatusOpen {
		return fmt.Errorf("round %d is not open (status: %s)", sub.RoundID, status)
	}
	if time.Now().UTC().After(deadline) {
		return fmt.Errorf("round %d deadline has passed", sub.RoundID)
	}

	// Upsert the participant record (idempotent — node can re-submit).
	_, err = o.db.Exec(ctx,
		`INSERT INTO fl_participants
		    (round_number, node_id, gradient_hash, submitted_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (round_number, node_id)
		 DO UPDATE SET gradient_hash = EXCLUDED.gradient_hash,
		               submitted_at  = EXCLUDED.submitted_at`,
		sub.RoundID, sub.NodeID, sub.GradientHash,
	)
	if err != nil {
		return fmt.Errorf("recording gradient submission: %w", err)
	}

	o.log.Info("gradient submitted",
		zap.Int32("round_number", sub.RoundID),
		zap.String("node_id", sub.NodeID.String()),
		zap.String("gradient_hash", sub.GradientHash),
	)
	return nil
}

// TryAggregate checks whether the round has enough participants to aggregate.
// If so, it transitions the round to "aggregating", performs FedAvg, and
// marks the round complete. Safe to call from a ticker without external locking
// because the DB UPDATE is conditional on status = 'open'.
func (o *Orchestrator) TryAggregate(ctx context.Context, roundNumber int32) (*RoundSummary, error) {
	// Count current submissions.
	var count int
	if err := o.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM fl_participants WHERE round_number = $1`, roundNumber,
	).Scan(&count); err != nil {
		return nil, fmt.Errorf("counting participants: %w", err)
	}
	if count < o.minParticipants {
		return nil, nil // not ready yet
	}

	// Transition to aggregating (atomic CAS on status).
	tag, err := o.db.Exec(ctx,
		`UPDATE fl_rounds SET status = 'aggregating'
		 WHERE round_number = $1 AND status = 'open'`,
		roundNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("transitioning round to aggregating: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Another goroutine already picked it up — not an error.
		return nil, nil
	}

	o.log.Info("aggregating FL round", zap.Int32("round_number", roundNumber), zap.Int("participants", count))

	// Fetch gradient hashes for anomaly detection and aggregation.
	gradHashes, err := o.fetchGradientHashes(ctx, roundNumber)
	if err != nil {
		_ = o.failRound(ctx, roundNumber, "fetching gradients: "+err.Error())
		return nil, fmt.Errorf("fetching gradient hashes: %w", err)
	}

	// Anomaly detection — discard outliers (SRS-FL-06).
	accepted := o.anomalyFilter(gradHashes)
	if len(accepted) < o.minParticipants {
		_ = o.failRound(ctx, roundNumber, "too many anomalous gradients discarded")
		return nil, fmt.Errorf("round %d failed anomaly filter: only %d/%d accepted", roundNumber, len(accepted), count)
	}

	// FedAvg aggregation — scaffold placeholder (real impl: load tensors, average).
	aggHash := o.fedAvg(ctx, roundNumber, accepted)

	// Mark round complete and persist aggregated model hash.
	if err := o.completeRound(ctx, roundNumber, aggHash); err != nil {
		return nil, fmt.Errorf("completing round: %w", err)
	}

	summary := &RoundSummary{
		RoundID:          roundNumber,
		ParticipantCount: len(accepted),
		AggregatedHash:   aggHash,
		OTSPending:       true, // OTS stamping is async
		CompletedAt:      time.Now().UTC(),
	}
	o.log.Info("FL round complete",
		zap.Int32("round_number", roundNumber),
		zap.Int("accepted_participants", len(accepted)),
		zap.String("aggregated_hash", aggHash),
	)
	return summary, nil
}

// CloseExpired finds open rounds past their deadline and fails them if
// they have insufficient participants. Called from a periodic ticker.
func (o *Orchestrator) CloseExpired(ctx context.Context) (int, error) {
	rows, err := o.db.Query(ctx,
		`SELECT round_number FROM fl_rounds
		 WHERE status = 'open' AND deadline_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("querying expired rounds: %w", err)
	}
	defer rows.Close()

	var closed int
	for rows.Next() {
		var rn int32
		if err := rows.Scan(&rn); err != nil {
			continue
		}
		// Try to aggregate; if not enough participants, fail.
		summary, err := o.TryAggregate(ctx, rn)
		if err != nil || summary == nil {
			_ = o.failRound(ctx, rn, "expired with insufficient participants")
			closed++
		}
	}
	return closed, rows.Err()
}

// GetRoundStatus returns the current status of a round.
func (o *Orchestrator) GetRoundStatus(ctx context.Context, roundNumber int32) (RoundStatus, error) {
	var status RoundStatus
	err := o.db.QueryRow(ctx,
		`SELECT status FROM fl_rounds WHERE round_number = $1`, roundNumber,
	).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("fetching round status: %w", err)
	}
	return status, nil
}

// ─── private helpers ────────────────────────────────────────────────────────

type gradientRecord struct {
	NodeID uuid.UUID
	Hash   string
}

func (o *Orchestrator) fetchGradientHashes(ctx context.Context, roundNumber int32) ([]gradientRecord, error) {
	rows, err := o.db.Query(ctx,
		`SELECT node_id, gradient_hash FROM fl_participants WHERE round_number = $1`,
		roundNumber,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []gradientRecord
	for rows.Next() {
		var r gradientRecord
		if err := rows.Scan(&r.NodeID, &r.Hash); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// anomalyFilter removes gradient submissions whose hashes are statistical
// outliers. Scaffold: returns all records (real impl uses cosine similarity
// or norm-based detection on the actual tensors, SRS-FL-06).
func (o *Orchestrator) anomalyFilter(records []gradientRecord) []gradientRecord {
	// TODO(fl): implement norm-based outlier detection on actual gradient tensors.
	return records
}

// fedAvg performs Federated Averaging over the accepted gradient updates.
// Scaffold: returns a deterministic placeholder hash.
// Real implementation loads sparse gradient tensors, averages them, applies
// the result to the global model checkpoint, and returns SHA-256 of new weights.
func (o *Orchestrator) fedAvg(_ context.Context, roundNumber int32, records []gradientRecord) string {
	// TODO(fl): load and average gradient tensors; write new checkpoint.
	_ = records
	return fmt.Sprintf("fedavg-placeholder-round-%d", roundNumber)
}

func (o *Orchestrator) completeRound(ctx context.Context, roundNumber int32, aggHash string) error {
	_, err := o.db.Exec(ctx,
		`UPDATE fl_rounds
		 SET status = 'complete', aggregated_hash = $2, completed_at = now()
		 WHERE round_number = $1`,
		roundNumber, aggHash,
	)
	return err
}

func (o *Orchestrator) failRound(ctx context.Context, roundNumber int32, reason string) error {
	_, err := o.db.Exec(ctx,
		`UPDATE fl_rounds SET status = 'failed', failure_reason = $2 WHERE round_number = $1`,
		roundNumber, reason,
	)
	if err != nil {
		o.log.Error("failed to mark round as failed",
			zap.Int32("round_number", roundNumber),
			zap.String("reason", reason),
			zap.Error(err),
		)
	}
	return err
}
