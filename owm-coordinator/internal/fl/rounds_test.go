package fl_test

import (
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/fl"
)

// TestRoundStatusConstants verifies that FL round status values are stable strings.
func TestRoundStatusConstants(t *testing.T) {
	cases := []struct {
		status fl.RoundStatus
		want   string
	}{
		{fl.RoundStatusOpen, "open"},
		{fl.RoundStatusAggregating, "aggregating"},
		{fl.RoundStatusComplete, "complete"},
		{fl.RoundStatusFailed, "failed"},
	}
	for _, c := range cases {
		if string(c.status) != c.want {
			t.Errorf("RoundStatus %q: got %q, want %q", c.status, c.status, c.want)
		}
	}
}

// TestRoundSummaryZeroValue confirms RoundSummary has safe zero values.
func TestRoundSummaryZeroValue(t *testing.T) {
	var s fl.RoundSummary
	if s.ParticipantCount != 0 {
		t.Errorf("expected 0 participants in zero-value RoundSummary, got %d", s.ParticipantCount)
	}
	if s.OTSPending {
		t.Error("expected OTSPending=false for zero-value RoundSummary")
	}
}

// TestGradientSubmissionZeroValue ensures GradientSubmission can be constructed safely.
func TestGradientSubmissionZeroValue(t *testing.T) {
	sub := fl.GradientSubmission{}
	if sub.GradientHash != "" {
		t.Errorf("expected empty GradientHash for zero-value, got %q", sub.GradientHash)
	}
}

// Integration tests require a live PostgreSQL instance.
func TestOpenRoundIntegration(t *testing.T) {
	t.Skip("integration test — set OWM_TEST_DSN and remove t.Skip to run")
}

func TestSubmitGradientIntegration(t *testing.T) {
	t.Skip("integration test — set OWM_TEST_DSN and remove t.Skip to run")
}

func TestTryAggregateIntegration(t *testing.T) {
	t.Skip("integration test — set OWM_TEST_DSN and remove t.Skip to run")
}
