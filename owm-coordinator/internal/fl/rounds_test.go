package fl_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"

	"github.com/owmnetwork/owm-coordinator/internal/fl"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
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
// Run with: OWM_TEST_DSN=postgres://... go test ./internal/fl/...

func TestOpenRoundIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	o := fl.New(pool, nil, nil, testutil.Logger())

	roundNumber, err := o.OpenRound(ctx, "owm-model-v1")
	if err != nil {
		t.Fatalf("OpenRound: %v", err)
	}
	if roundNumber <= 0 {
		t.Errorf("round_number: got %d, want > 0", roundNumber)
	}

	status, err := o.GetRoundStatus(ctx, roundNumber)
	if err != nil {
		t.Fatalf("GetRoundStatus: %v", err)
	}
	if status != fl.RoundStatusOpen {
		t.Errorf("status: got %q, want %q", status, fl.RoundStatusOpen)
	}
}

func TestSubmitGradientIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	o := fl.New(pool, nil, nil, testutil.Logger())

	roundNumber, err := o.OpenRound(ctx, "owm-model-v1")
	if err != nil {
		t.Fatalf("OpenRound: %v", err)
	}

	// fl_participants.node_id references nodes, so insert a real node first.
	nodeID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'active')`,
		nodeID, "pk-submit-"+nodeID.String(), nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	gradientData := make([]byte, 64)
	h := sha256.Sum256(gradientData)
	err = o.SubmitGradient(ctx, fl.GradientSubmission{
		NodeID:       nodeID,
		RoundID:      roundNumber,
		GradientData: gradientData,
		GradientHash: hex.EncodeToString(h[:]),
		S3URL:        "gradients/1/test.bin",
	})
	if err != nil {
		t.Fatalf("SubmitGradient: %v", err)
	}

	var count int
	_ = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM fl_participants fp
		 JOIN fl_rounds r ON r.round_id = fp.round_id
		 WHERE r.round_number = $1 AND fp.node_id = $2`,
		roundNumber, nodeID,
	).Scan(&count)
	if count != 1 {
		t.Errorf("fl_participants: got %d row(s), want 1", count)
	}
}

func TestTryAggregateIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	// Use a fast-failing OTS calendar so the async stampOTS goroutine exits quickly.
	o := fl.NewWithConfig(pool, nil, nil, &fl.OrchestratorConfig{
		OTSCalendars: []string{"http://127.0.0.1:1/digest"},
	}, testutil.Logger())

	roundNumber, err := o.OpenRound(ctx, "owm-model-v1")
	if err != nil {
		t.Fatalf("OpenRound: %v", err)
	}

	// Submit gradients from 3 nodes (minParticipants = 3).
	for i := 0; i < 3; i++ {
		nodeID := uuid.New()
		_, err = pool.Exec(ctx,
			`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
			 VALUES ($1, $2, $3, 't1', 'active')`,
			nodeID,
			"pk-agg-"+nodeID.String(),
			nodeID.String()+"@127.0.0.1:9735",
		)
		if err != nil {
			t.Fatalf("insert node %d: %v", i, err)
		}
		data := make([]byte, 64)
		data[0] = byte(i)
		h := sha256.Sum256(data)
		if err := o.SubmitGradient(ctx, fl.GradientSubmission{
			NodeID:       nodeID,
			RoundID:      roundNumber,
			GradientHash: hex.EncodeToString(h[:]),
		}); err != nil {
			t.Fatalf("SubmitGradient %d: %v", i, err)
		}
	}

	summary, err := o.TryAggregate(ctx, roundNumber)
	if err != nil {
		t.Fatalf("TryAggregate: %v", err)
	}
	if summary == nil {
		t.Fatal("TryAggregate: got nil summary, expected aggregation to proceed")
	}
	if summary.ParticipantCount != 3 {
		t.Errorf("ParticipantCount: got %d, want 3", summary.ParticipantCount)
	}
	if summary.AggregatedHash == "" {
		t.Error("AggregatedHash: expected non-empty")
	}

	status, err := o.GetRoundStatus(ctx, roundNumber)
	if err != nil {
		t.Fatalf("GetRoundStatus: %v", err)
	}
	if status != fl.RoundStatusComplete {
		t.Errorf("round status: got %q, want %q", status, fl.RoundStatusComplete)
	}
}
