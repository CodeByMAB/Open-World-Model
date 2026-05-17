// Package fl orchestrates federated learning rounds across participating nodes
// (SRS-FL-01 through SRS-FL-07). Each round collects gradient updates, runs
// anomaly detection, performs FedAvg aggregation, and triggers OTS anchoring.
package fl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/metrics"
	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/storage"
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
	RoundID      int32  // round_number (display/API); fl_participants uses round_id internally
	GradientData []byte
	GradientHash string // SHA-256 hex of gradient_data
	S3URL        string // S3 object key only (SSRF-safe), e.g. "gradients/1/uuid.bin"
}

// RoundSummary is returned after a round completes aggregation.
type RoundSummary struct {
	RoundID        int32
	ParticipantCount int
	AggregatedHash string // SHA-256 of the aggregated weights
	OTSPending     bool   // true until Bitcoin anchoring confirms
	CompletedAt    time.Time
}

// defaultOTSCalendars are the three standard OpenTimestamps calendar servers (SRS-OTS-02).
var defaultOTSCalendars = []string{
	"https://a.pool.opentimestamps.org/digest",
	"https://b.pool.opentimestamps.org/digest",
	"https://c.pool.opentimestamps.org/digest",
}

// OrchestratorConfig holds FL tuning parameters.
type OrchestratorConfig struct {
	GradientL2ClipNorm     float64
	AnomalyStdDevThreshold float64
	// OTSCalendars overrides the default three OTS calendar URLs (SRS-OTS-02).
	// If empty, defaultOTSCalendars is used.
	OTSCalendars []string
}

// Orchestrator manages FL round lifecycle.
type Orchestrator struct {
	db       *pgxpool.Pool
	registry *registry.Registry
	storage  *storage.S3Client
	log      *zap.Logger

	minParticipants        int
	roundDurationSecs      int
	gradientL2ClipNorm     float64
	anomalyStdDevThreshold float64

	otsCalendars []string
	otsClient    *http.Client
}

// New creates an FL Orchestrator with sensible defaults.
func New(db *pgxpool.Pool, reg *registry.Registry, s3 *storage.S3Client, log *zap.Logger) *Orchestrator {
	return NewWithConfig(db, reg, s3, nil, log)
}

// NewWithConfig creates an FL Orchestrator with optional FL config.
func NewWithConfig(db *pgxpool.Pool, reg *registry.Registry, s3 *storage.S3Client, flCfg *OrchestratorConfig, log *zap.Logger) *Orchestrator {
	clipNorm := 1.0
	anomThreshold := 3.0
	calendars := defaultOTSCalendars
	if flCfg != nil {
		if flCfg.GradientL2ClipNorm > 0 {
			clipNorm = flCfg.GradientL2ClipNorm
		}
		if flCfg.AnomalyStdDevThreshold > 0 {
			anomThreshold = flCfg.AnomalyStdDevThreshold
		}
		if len(flCfg.OTSCalendars) > 0 {
			calendars = flCfg.OTSCalendars
		}
	}
	return &Orchestrator{
		db:                     db,
		registry:               reg,
		storage:                s3,
		log:                    log,
		minParticipants:        3,
		roundDurationSecs:      3600,
		gradientL2ClipNorm:     clipNorm,
		anomalyStdDevThreshold: anomThreshold,
		otsCalendars:           calendars,
		otsClient:              &http.Client{Timeout: 30 * time.Second},
	}
}

// OpenRound creates a new FL round record and returns the round number.
// Only one round should be open at a time; callers must check before calling.
func (o *Orchestrator) OpenRound(ctx context.Context, modelVersionID string) (int32, error) {
	var roundNumber int32
	err := o.db.QueryRow(ctx,
		`INSERT INTO fl_rounds (model_version, status, started_at)
		 VALUES ($1, 'open', now())
		 RETURNING round_number`,
		modelVersionID,
	).Scan(&roundNumber)
	if err != nil {
		return 0, fmt.Errorf("opening FL round: %w", err)
	}
	o.log.Info("FL round opened",
		zap.Int32("round_number", roundNumber),
		zap.String("model_version", modelVersionID),
	)
	return roundNumber, nil
}

// SubmitGradient records a gradient update from a node for the current round.
// Returns an error if the round is not open or the node is not eligible.
func (o *Orchestrator) SubmitGradient(ctx context.Context, sub GradientSubmission) error {
	// Resolve round_number -> round_id (fl_participants uses round_id as FK/PK).
	var roundID int
	var status RoundStatus
	err := o.db.QueryRow(ctx,
		`SELECT round_id, status FROM fl_rounds WHERE round_number = $1`,
		sub.RoundID,
	).Scan(&roundID, &status)
	if err != nil {
		return fmt.Errorf("fetching round %d: %w", sub.RoundID, err)
	}
	if status != RoundStatusOpen {
		return fmt.Errorf("round %d is not open (status: %s)", sub.RoundID, status)
	}

	// Upsert the participant record (idempotent — node can re-submit).
	_, err = o.db.Exec(ctx,
		`INSERT INTO fl_participants
		    (round_id, node_id, gradient_hash, s3_url)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (round_id, node_id)
		 DO UPDATE SET gradient_hash = EXCLUDED.gradient_hash,
		               s3_url = EXCLUDED.s3_url`,
		roundID, sub.NodeID, sub.GradientHash, sub.S3URL,
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
	var roundID int
	if err := o.db.QueryRow(ctx,
		`SELECT round_id FROM fl_rounds WHERE round_number = $1`, roundNumber,
	).Scan(&roundID); err != nil {
		return nil, fmt.Errorf("resolving round: %w", err)
	}
	// Count current submissions (fl_participants uses round_id).
	var count int
	if err := o.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM fl_participants WHERE round_id = $1`, roundID,
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
	gradHashes, err := o.fetchGradientHashes(ctx, roundID, roundNumber)
	if err != nil {
		_ = o.failRound(ctx, roundNumber, "fetching gradients: "+err.Error())
		return nil, fmt.Errorf("fetching gradient hashes: %w", err)
	}

	// Anomaly detection — discard outliers (SRS-FL-06).
	accepted, err := o.anomalyFilter(ctx, roundID, gradHashes)
	if err != nil {
		_ = o.failRound(ctx, roundNumber, "anomaly filter: "+err.Error())
		return nil, fmt.Errorf("anomaly filter: %w", err)
	}
	if len(accepted) < o.minParticipants {
		_ = o.failRound(ctx, roundNumber, "too many anomalous gradients discarded")
		return nil, fmt.Errorf("round %d failed anomaly filter: only %d/%d accepted", roundNumber, len(accepted), count)
	}

	// FedAvg aggregation.
	aggHash, err := o.fedAvg(ctx, roundNumber, accepted)
	if err != nil {
		_ = o.failRound(ctx, roundNumber, "fedAvg: "+err.Error())
		return nil, fmt.Errorf("fedAvg: %w", err)
	}

	// Mark round complete and persist aggregated model hash.
	if err := o.completeRound(ctx, roundNumber, aggHash); err != nil {
		return nil, fmt.Errorf("completing round: %w", err)
	}
	// Async OTS stamping (detached from request context).
	go o.stampOTS(aggHash, roundNumber)

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
		 WHERE status = 'open' AND started_at + ($1 || ' seconds')::INTERVAL < now()`,
		o.roundDurationSecs,
	)
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
	S3URL  string
}

func (o *Orchestrator) fetchGradientHashes(ctx context.Context, roundID int, roundNumber int32) ([]gradientRecord, error) {
	rows, err := o.db.Query(ctx,
		`SELECT node_id, gradient_hash, s3_url FROM fl_participants WHERE round_id = $1`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []gradientRecord
	for rows.Next() {
		var r gradientRecord
		if err := rows.Scan(&r.NodeID, &r.Hash, &r.S3URL); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// anomalyFilter removes gradient submissions that are statistical outliers (L2 norm).
func (o *Orchestrator) anomalyFilter(ctx context.Context, roundID int, records []gradientRecord) ([]gradientRecord, error) {
	if o.storage == nil {
		return records, nil
	}
	type gradMap = map[string]interface{}
	var norms []float64
	normByNode := make(map[uuid.UUID]float64)
	for _, r := range records {
		if r.S3URL == "" {
			continue
		}
		blob, err := o.storage.GetObject(ctx, r.S3URL)
		if err != nil {
			o.log.Warn("anomaly filter: failed to fetch gradient", zap.String("s3_url", r.S3URL), zap.Error(err))
			continue
		}
		var m gradMap
		if err := msgpack.Unmarshal(blob, &m); err != nil {
			o.log.Warn("anomaly filter: failed to decode msgpack", zap.Error(err))
			continue
		}
		norm := l2NormFromMap(m)
		norms = append(norms, norm)
		normByNode[r.NodeID] = norm
	}
	if len(norms) < 2 {
		return records, nil
	}
	median := medianFloat64(norms)
	stddev := stddevFloat64(norms, median)
	threshold := median + o.anomalyStdDevThreshold*stddev
	var accepted []gradientRecord
	for _, r := range records {
		n, ok := normByNode[r.NodeID]
		if !ok {
			accepted = append(accepted, r)
			continue
		}
		if n > threshold {
			_, _ = o.db.Exec(ctx,
				`UPDATE fl_participants SET anomaly_flagged = true WHERE round_id = $1 AND node_id = $2`,
				roundID, r.NodeID,
			)
			continue
		}
		accepted = append(accepted, r)
	}
	return accepted, nil
}

// fedAvg performs Federated Averaging: fetch gradients from S3, clip, weight by tier/bonus, average, upload checkpoint.
func (o *Orchestrator) fedAvg(ctx context.Context, roundNumber int32, records []gradientRecord) (string, error) {
	if o.storage == nil {
		return fmt.Sprintf("fedavg-placeholder-round-%d", roundNumber), nil
	}
	type gradMap = map[string][]float32
	clipNorm := o.gradientL2ClipNorm
	if clipNorm <= 0 {
		clipNorm = 1.0
	}

	var weights []float64
	var grads []gradMap
	for _, r := range records {
		blob, err := o.storage.GetObject(ctx, r.S3URL)
		if err != nil {
			return "", fmt.Errorf("fetch gradient %s: %w", r.S3URL, err)
		}
		var raw map[string]interface{}
		if err := msgpack.Unmarshal(blob, &raw); err != nil {
			return "", fmt.Errorf("decode msgpack: %w", err)
		}
		m := toGradMap(raw)
		clipL2(m, clipNorm)
		grads = append(grads, m)
		w := 1.0
		_ = o.db.QueryRow(ctx,
			`SELECT COALESCE(ns.bonus_multiplier, 1.0) * CASE n.tier WHEN 't2' THEN 2.5 WHEN 't3' THEN 8.0 ELSE 1.0 END
			 FROM nodes n LEFT JOIN node_stakes ns ON ns.node_id = n.node_id WHERE n.node_id = $1`,
			r.NodeID,
		).Scan(&w)
		weights = append(weights, w)
	}
	totalWeight := 0.0
	for _, w := range weights {
		totalWeight += w
	}
	if totalWeight <= 0 {
		totalWeight = 1
	}

	avg := weightedAverage(grads, weights, totalWeight)
	checkpointBytes, err := msgpack.Marshal(avg)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(checkpointBytes)
	hashHex := hex.EncodeToString(hash[:])
	key := "models/owm-v" + strconv.Itoa(int(roundNumber)) + ".bin"
	if _, err := o.storage.PutObject(ctx, key, checkpointBytes); err != nil {
		return "", fmt.Errorf("upload checkpoint: %w", err)
	}
	return hashHex, nil
}

func l2NormFromMap(m map[string]interface{}) float64 {
	var sum float64
	for _, v := range m {
		if arr, ok := v.([]interface{}); ok {
			for _, x := range arr {
				if f, ok := toFloat64(x); ok {
					sum += f * f
				}
			}
		}
	}
	return math.Sqrt(sum)
}

func toFloat64(x interface{}) (float64, bool) {
	switch v := x.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	default:
		return 0, false
	}
}

func medianFloat64(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	b := make([]float64, len(a))
	copy(b, a)
	for i := 0; i < len(b); i++ {
		for j := i + 1; j < len(b); j++ {
			if b[j] < b[i] {
				b[i], b[j] = b[j], b[i]
			}
		}
	}
	mid := len(b) / 2
	if len(b)%2 == 1 {
		return b[mid]
	}
	return (b[mid-1] + b[mid]) / 2
}

func stddevFloat64(a []float64, mean float64) float64 {
	if len(a) < 2 {
		return 0
	}
	var sum float64
	for _, x := range a {
		d := x - mean
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(a)-1))
}

func toGradMap(raw map[string]interface{}) map[string][]float32 {
	out := make(map[string][]float32)
	for k, v := range raw {
		if arr, ok := v.([]interface{}); ok {
			fs := make([]float32, len(arr))
			for i, x := range arr {
				if f, ok := toFloat64(x); ok {
					fs[i] = float32(f)
				}
			}
			out[k] = fs
		}
	}
	return out
}

func clipL2(m map[string][]float32, maxNorm float64) {
	norm := 0.0
	for _, arr := range m {
		for _, v := range arr {
			norm += float64(v) * float64(v)
		}
	}
	norm = math.Sqrt(norm)
	if norm <= maxNorm || norm == 0 {
		return
	}
	scale := maxNorm / norm
	for k, arr := range m {
		for i := range arr {
			arr[i] = float32(float64(arr[i]) * scale)
		}
		m[k] = arr
	}
}

func weightedAverage(grads []map[string][]float32, weights []float64, totalWeight float64) map[string][]float32 {
	if len(grads) == 0 {
		return nil
	}
	out := make(map[string][]float32)
	keysSeen := make(map[string]struct{})
	for _, g := range grads {
		for key := range g {
			keysSeen[key] = struct{}{}
		}
	}
	for key := range keysSeen {
		var sum []float32
		for i, g := range grads {
			arr := g[key]
			if arr == nil {
				continue
			}
			if sum == nil {
				sum = make([]float32, len(arr))
			}
			w := weights[i] / totalWeight
			for j := range arr {
				if j < len(sum) {
					sum[j] += float32(w) * arr[j]
				}
			}
		}
		if sum != nil {
			out[key] = sum
		}
	}
	return out
}

func (o *Orchestrator) completeRound(ctx context.Context, roundNumber int32, aggHash string) error {
	_, err := o.db.Exec(ctx,
		`UPDATE fl_rounds
		 SET status = 'complete', checkpoint_hash = $2, completed_at = now()
		 WHERE round_number = $1`,
		roundNumber, aggHash,
	)
	if err == nil {
		metrics.OwmFLRoundsTotal.WithLabelValues("complete").Inc()
	}
	return err
}

// stampOTS submits the aggregated checkpoint hash to all configured OTS calendar servers
// concurrently (SRS-OTS-01, SRS-OTS-02), stores the first returned binary proof in S3
// under ots/round-{N}.ots, and records that key in fl_rounds. Called in a detached goroutine.
func (o *Orchestrator) stampOTS(hashHex string, roundNumber int32) {
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil || len(hashBytes) != sha256.Size {
		o.log.Warn("OTS stamp: invalid hash", zap.String("hash", hashHex))
		return
	}
	if len(o.otsCalendars) == 0 {
		o.log.Warn("OTS stamp: no calendars configured, skipping")
		return
	}

	type calResult struct {
		proof []byte
		url   string
		err   error
	}
	results := make(chan calResult, len(o.otsCalendars))
	for _, calURL := range o.otsCalendars {
		calURL := calURL
		go func() {
			proof, err := o.submitToOTSCalendar(calURL, hashBytes)
			results <- calResult{proof: proof, url: calURL, err: err}
		}()
	}

	var proofBytes []byte
	successCount := 0
	for range o.otsCalendars {
		res := <-results
		if res.err != nil {
			o.log.Warn("OTS stamp: calendar failed",
				zap.String("calendar", res.url), zap.Error(res.err))
			continue
		}
		successCount++
		o.log.Info("OTS stamp: calendar accepted",
			zap.String("calendar", res.url), zap.Int("proof_bytes", len(res.proof)))
		if proofBytes == nil {
			proofBytes = res.proof
		}
	}

	if successCount == 0 {
		o.log.Warn("OTS stamp: all calendars failed, proof not stored",
			zap.String("hash", hashHex), zap.Int32("round", roundNumber))
		return
	}

	s3Key := fmt.Sprintf("ots/round-%d.ots", roundNumber)
	if o.storage != nil && len(proofBytes) > 0 {
		if _, err := o.storage.PutObject(context.Background(), s3Key, proofBytes); err != nil {
			o.log.Warn("OTS stamp: S3 upload failed",
				zap.String("key", s3Key), zap.Error(err))
			// Still record the key — the path marks the intent and can be retried.
		}
	}

	if _, err := o.db.Exec(context.Background(),
		`UPDATE fl_rounds SET ots_proof_path = $2 WHERE round_number = $1`,
		roundNumber, s3Key,
	); err != nil {
		o.log.Warn("OTS stamp: DB update failed", zap.Error(err))
		return
	}
	o.log.Info("OTS stamp: proof recorded",
		zap.Int32("round", roundNumber),
		zap.String("path", s3Key),
		zap.Int("calendars_succeeded", successCount),
	)
}

// submitToOTSCalendar POSTs hashBytes to one OTS calendar and returns the binary proof.
func (o *Orchestrator) submitToOTSCalendar(calURL string, hashBytes []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, calURL, bytes.NewReader(hashBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := o.otsClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	proof, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // 64 KB cap
	if err != nil {
		return nil, fmt.Errorf("reading proof: %w", err)
	}
	if len(proof) == 0 {
		return nil, fmt.Errorf("empty proof body")
	}
	return proof, nil
}

func (o *Orchestrator) failRound(ctx context.Context, roundNumber int32, reason string) error {
	_, err := o.db.Exec(ctx,
		`UPDATE fl_rounds SET status = 'failed' WHERE round_number = $1`,
		roundNumber,
	)
	if err != nil {
		o.log.Error("failed to mark round as failed",
			zap.Int32("round_number", roundNumber),
			zap.String("reason", reason),
			zap.Error(err),
		)
	} else {
		metrics.OwmFLRoundsTotal.WithLabelValues("failed").Inc()
	}
	return err
}
