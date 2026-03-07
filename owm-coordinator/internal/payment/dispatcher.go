// Package payment implements async Lightning payment dispatch for task rewards.
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/metrics"
	"github.com/owmnetwork/owm-coordinator/internal/observer"
)

const queueSize = 1000
const numWorkers = 10

type paymentJob struct {
	TaskID     uuid.UUID
	NodeLNURI  string
	AmountSats int64
}

// Dispatcher processes task reward payments asynchronously via a worker pool.
type Dispatcher struct {
	ln       lightning.Client
	db       *pgxpool.Pool
	rdb      *redis.Client
	observer *observer.Client
	queue    chan paymentJob
	log      *zap.Logger
}

// New creates a Dispatcher. Queue size is 1000; if full, enqueue drops and logs.
// If obs is non-nil, successful payments are submitted to the Observer Registry
// asynchronously (fire-and-forget); failures are logged but do not affect payment status.
func New(ln lightning.Client, db *pgxpool.Pool, rdb *redis.Client, log *zap.Logger, obs *observer.Client) *Dispatcher {
	return &Dispatcher{
		ln:       ln,
		db:       db,
		rdb:      rdb,
		observer: obs,
		queue:    make(chan paymentJob, queueSize),
		log:      log,
	}
}

// Start runs startup recovery (requeue pending from DB) and launches worker goroutines.
func (d *Dispatcher) Start(ctx context.Context) {
	// Startup recovery: requeue pending payments.
	rows, err := d.db.Query(ctx,
		`SELECT task_id, node_ln_uri, reward_sats FROM tasks WHERE payment_status = 'pending' AND status = 'completed'`)
	if err != nil {
		d.log.Warn("payment recovery scan failed", zap.Error(err))
	} else {
		for rows.Next() {
			var j paymentJob
			if err := rows.Scan(&j.TaskID, &j.NodeLNURI, &j.AmountSats); err != nil {
				continue
			}
			select {
			case d.queue <- j:
			default:
				d.log.Warn("payment queue full at startup, skipping recovery job", zap.String("task_id", j.TaskID.String()))
			}
		}
		rows.Close()
	}

	for i := 0; i < numWorkers; i++ {
		go d.worker(ctx)
	}
}

// Enqueue adds a payment job. Non-blocking; if queue is full, logs and returns.
func (d *Dispatcher) Enqueue(taskID uuid.UUID, nodeLNURI string, amountSats int64) {
	j := paymentJob{TaskID: taskID, NodeLNURI: nodeLNURI, AmountSats: amountSats}
	select {
	case d.queue <- j:
	default:
		d.log.Warn("payment queue full, dropping", zap.String("task_id", taskID.String()))
	}
}

func (d *Dispatcher) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-d.queue:
			d.process(ctx, job)
		}
	}
}

func (d *Dispatcher) process(ctx context.Context, job paymentJob) {
	pubkeyHex := parsePubkeyFromLNURI(job.NodeLNURI)
	req := lightning.SendPaymentRequest{
		DestPubkeyHex: pubkeyHex,
		AmountSats:    job.AmountSats,
		TimeoutSecs:   60,
	}

	var lastErr error
	delays := []time.Duration{0, 5 * time.Second, 30 * time.Second, 120 * time.Second}
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delays[attempt]):
			}
		}
		result, err := d.ln.SendPayment(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		if result != nil && result.Status == "SUCCEEDED" {
			preimage := result.Preimage
			tag, err := d.db.Exec(ctx,
				`UPDATE tasks SET payment_status = 'paid', payment_preimage = $2 WHERE task_id = $1 AND payment_status = 'pending'`,
				job.TaskID, preimage,
			)
			if err != nil {
				d.log.Error("payment DB update failed", zap.String("task_id", job.TaskID.String()), zap.Error(err))
				return
			}
			if tag.RowsAffected() == 0 {
				d.log.Warn("duplicate payment detected", zap.String("task_id", job.TaskID.String()))
			}
			metrics.OwmPaymentsTotal.WithLabelValues("paid").Inc()
			metrics.OwmPaymentSatsTotal.Add(float64(job.AmountSats))
			// Observer Protocol: submit receipt asynchronously; do not block or retry on failure.
			if d.observer != nil {
				go d.submitObserverReceipt(context.Background(), job.TaskID, job.NodeLNURI, job.AmountSats, preimage)
			}
			return
		}
		if result != nil && result.Status == "FAILED" {
			lastErr = fmt.Errorf("payment failed: %s", result.FailureReason)
		}
	}

	_, _ = d.db.Exec(ctx,
		`UPDATE tasks SET payment_status = 'failed' WHERE task_id = $1 AND payment_status = 'pending'`,
		job.TaskID,
	)
	metrics.OwmPaymentsTotal.WithLabelValues("failed").Inc()
	d.log.Error("payment failed after retries",
		zap.String("task_id", job.TaskID.String()),
		zap.Int64("amount_sats", job.AmountSats),
		zap.Error(lastErr),
	)
}

// submitObserverReceipt builds an Observer receipt and submits it in a goroutine.
// Idempotent: skips if task already has observer_receipt_id. On success, updates
// tasks.observer_receipt_id (and only counts success when exactly one row is updated).
// On submission failure, logs the full receipt payload as JSON at WARN level in a
// single structured field (receipt_payload) for manual resubmission and audit.
// Failures are reflected in metrics only; payment status is unchanged.
func (d *Dispatcher) submitObserverReceipt(ctx context.Context, taskID uuid.UUID, nodeLNURI string, amountSats int64, preimage string) {
	var existingID string
	err := d.db.QueryRow(ctx, `SELECT observer_receipt_id FROM tasks WHERE task_id = $1`, taskID).Scan(&existingID)
	if err == nil && existingID != "" {
		metrics.OwmObserverReceiptsTotal.WithLabelValues("skipped").Inc()
		d.log.Debug("observer receipt already present, skipping", zap.String("task_id", taskID.String()), zap.String("observer_receipt_id", existingID))
		return
	}

	receipt := observer.Receipt{
		PaymentRail:           observer.PaymentRailLightning,
		SettlementReference:   preimage,
		ReceiverPublicKeyHash: observer.PubkeyHashHex(parsePubkeyFromLNURI(nodeLNURI)),
		AmountBucket:          observer.AmountBucket(amountSats),
	}
	receiptID, err := d.observer.Submit(ctx, receipt)
	if err != nil {
		metrics.OwmObserverReceiptsTotal.WithLabelValues("failure").Inc()
		receiptPayload, _ := json.Marshal(receipt)
		d.log.Warn("observer receipt submission failed",
			zap.String("task_id", taskID.String()),
			zap.String("receipt_payload", string(receiptPayload)),
			zap.Error(err),
		)
		return
	}
	if receiptID == "" {
		metrics.OwmObserverReceiptsTotal.WithLabelValues("failure").Inc()
		return
	}
	tag, err := d.db.Exec(ctx,
		`UPDATE tasks SET observer_receipt_id = $2 WHERE task_id = $1`,
		taskID, receiptID,
	)
	if err != nil {
		metrics.OwmObserverReceiptsTotal.WithLabelValues("failure").Inc()
		d.log.Warn("failed to store observer_receipt_id",
			zap.String("task_id", taskID.String()),
			zap.String("receipt_id", receiptID),
			zap.Error(err),
		)
		return
	}
	if tag.RowsAffected() != 1 {
		metrics.OwmObserverReceiptsTotal.WithLabelValues("not_persisted").Inc()
		d.log.Warn("observer receipt submitted but task row not updated; receipt_id lost",
			zap.String("task_id", taskID.String()),
			zap.String("receipt_id", receiptID),
			zap.Int64("rows_affected", tag.RowsAffected()),
		)
		return
	}
	metrics.OwmObserverReceiptsTotal.WithLabelValues("success").Inc()
}

// parsePubkeyFromLNURI extracts the node pubkey (hex) from an LN node URI.
// Format is typically "pubkey@host:port"; the part before @ is the pubkey.
func parsePubkeyFromLNURI(lnURI string) string {
	if idx := strings.Index(lnURI, "@"); idx >= 0 {
		return strings.TrimSpace(lnURI[:idx])
	}
	return strings.TrimSpace(lnURI)
}
