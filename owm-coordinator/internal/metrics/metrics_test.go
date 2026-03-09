package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetrics_AllEightPresent(t *testing.T) {
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
	}

	for _, name := range names {
		if !strings.Contains(body, name) {
			t.Errorf("metric %s not found in /metrics output", name)
		}
	}
}
