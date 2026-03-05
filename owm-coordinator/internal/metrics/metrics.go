// Package metrics defines Prometheus metrics for the OWM coordinator.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// OwmNodesTotal is a gauge of nodes by tier and status.
	OwmNodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "owm_nodes_total", Help: "Number of registered nodes by tier and status"},
		[]string{"tier", "status"},
	)
	// OwmTasksTotal counts tasks by type and status.
	OwmTasksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "owm_tasks_total", Help: "Total tasks by task_type and status"},
		[]string{"task_type", "status"},
	)
	// OwmTaskDurationSeconds is the histogram of task execution duration.
	OwmTaskDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{Name: "owm_task_duration_seconds", Help: "Task execution duration in seconds"},
	)
	// OwmFLRoundsTotal counts FL rounds by status.
	OwmFLRoundsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "owm_fl_rounds_total", Help: "Total FL rounds by status"},
		[]string{"status"},
	)
	// OwmPaymentsTotal counts payments by status.
	OwmPaymentsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "owm_payments_total", Help: "Total payments by status"},
		[]string{"status"},
	)
	// OwmPaymentSatsTotal is total satoshis paid out.
	OwmPaymentSatsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "owm_payment_sats_total", Help: "Total satoshis paid"},
	)
	// OwmStakeVerificationsTotal counts stake checks by result.
	OwmStakeVerificationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "owm_stake_verifications_total", Help: "Stake verifications by result"},
		[]string{"result"},
	)
	// OwmSlashEventsTotal counts slash events by tier.
	OwmSlashEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "owm_slash_events_total", Help: "Slash events by tier"},
		[]string{"tier"},
	)
)

func init() {
	prometheus.MustRegister(
		OwmNodesTotal,
		OwmTasksTotal,
		OwmTaskDurationSeconds,
		OwmFLRoundsTotal,
		OwmPaymentsTotal,
		OwmPaymentSatsTotal,
		OwmStakeVerificationsTotal,
		OwmSlashEventsTotal,
	)
}
