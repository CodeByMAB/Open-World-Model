package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetrics_AllFourteenPresent(t *testing.T) {
	// Vec metrics only appear in /metrics output once at least one label
	// combination has been observed. Initialize a sentinel value for each so
	// the Gather call below emits all metric families.
	OwmNodesTotal.WithLabelValues("_test", "_test")
	OwmTasksTotal.WithLabelValues("_test", "_test")
	OwmFLRoundsTotal.WithLabelValues("_test")
	OwmPaymentsTotal.WithLabelValues("_test")
	OwmStakeVerificationsTotal.WithLabelValues("_test")
	OwmSlashEventsTotal.WithLabelValues("_test")
	OwmSlashingEventsTotal.WithLabelValues("_test")
	OwmObserverReceiptsTotal.WithLabelValues("_test")
	OwmRegistrationRateLimitedTotal.WithLabelValues("_test")

	ts := httptest.NewServer(promhttp.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("failed to GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	body := string(raw)

	names := []string{
		"owm_nodes_total",
		"owm_tasks_total",
		"owm_task_duration_seconds",
		"owm_fl_rounds_total",
		"owm_payments_total",
		"owm_payment_sats_total",
		"owm_stake_verifications_total",
		"owm_slash_events_total",
		"owm_observer_receipts_total",
		"owm_registration_rate_limited_total",
		"owm_staked_channels_total",
		"owm_staked_sats_total",
		"owm_degraded_nodes_total",
		"owm_slashing_events_total",
	}

	for _, name := range names {
		if !strings.Contains(body, name) {
			t.Errorf("metric %s not found in /metrics output", name)
		}
	}
}
