// Package payment implements async Lightning payment dispatch for task rewards.
package payment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/metrics"
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
	ln   lightning.Client
	db   *pgxpool.Pool
	rdb  *redis.Client
	queue chan paymentJob
	log   *zap.Logger
}

// New creates a Dispatcher. Queue size is 1000; if full, enqueue drops and logs.
func New(ln lightning.Client, db *pgxpool.Pool, rdb *redis.Client, log *zap.Logger) *Dispatcher {
	return &Dispatcher{
		ln:    ln,
		db:    db,
		rdb:   rdb,
		queue: make(chan paymentJob, queueSize),
		log:   log,
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

// parsePubkeyFromLNURI extracts the node pubkey (hex) from an LN node URI.
// Format is typically "pubkey@host:port"; the part before @ is the pubkey.
func parsePubkeyFromLNURI(lnURI string) string {
	if idx := strings.Index(lnURI, "@"); idx >= 0 {
		return strings.TrimSpace(lnURI[:idx])
	}
	return strings.TrimSpace(lnURI)
}
