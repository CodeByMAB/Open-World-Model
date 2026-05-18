package scheduler

import (
	"context"
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/registry"
)

// filterEligible is unexported; these tests live in package scheduler.

var noopScheduler = &Scheduler{}

func TestFilterEligible_TierOnly(t *testing.T) {
	nodes := []*registry.Node{
		{Tier: "t1"},
		{Tier: "t2"},
		{Tier: "t3"},
	}
	// TaskGradientAgg needs tier 2.
	got := noopScheduler.filterEligible(context.Background(), nodes, TaskGradientAgg, 2)
	if len(got) != 2 {
		t.Errorf("tier filter: got %d, want 2 (t2+t3)", len(got))
	}
}

func TestFilterEligible_EmptySupportedTypes_AcceptsAll(t *testing.T) {
	nodes := []*registry.Node{
		{Tier: "t1", SupportedTaskTypes: nil},
		{Tier: "t1", SupportedTaskTypes: []string{}},
	}
	// Empty/nil list means "accept all task types".
	got := noopScheduler.filterEligible(context.Background(), nodes, TaskInference, 1)
	if len(got) != 2 {
		t.Errorf("nil/empty supported types: got %d, want 2", len(got))
	}
}

func TestFilterEligible_TaskTypeMismatch(t *testing.T) {
	nodes := []*registry.Node{
		{Tier: "t1", SupportedTaskTypes: []string{"fl_round", "gradient_agg"}},
		{Tier: "t1", SupportedTaskTypes: []string{"inference"}},
	}
	got := noopScheduler.filterEligible(context.Background(), nodes, TaskInference, 1)
	if len(got) != 1 {
		t.Errorf("task type mismatch: got %d, want 1 (inference-only node)", len(got))
	}
	if got[0].SupportedTaskTypes[0] != "inference" {
		t.Errorf("wrong node selected: got %v", got[0].SupportedTaskTypes)
	}
}

func TestFilterEligible_TierAndTaskTypeCombined(t *testing.T) {
	nodes := []*registry.Node{
		// Passes tier but not task type.
		{Tier: "t2", SupportedTaskTypes: []string{"inference"}},
		// Passes both.
		{Tier: "t2", SupportedTaskTypes: []string{"gradient_agg"}},
		// Fails tier.
		{Tier: "t1", SupportedTaskTypes: []string{"gradient_agg"}},
		// Passes tier, empty list (all types).
		{Tier: "t3", SupportedTaskTypes: nil},
	}
	// TaskGradientAgg needs tier 2 — nodes[0] fails task type, nodes[2] fails tier.
	got := noopScheduler.filterEligible(context.Background(), nodes, TaskGradientAgg, 2)
	if len(got) != 2 {
		t.Errorf("combined filter: got %d, want 2", len(got))
	}
}

func TestFilterEligible_NoNodes(t *testing.T) {
	got := noopScheduler.filterEligible(context.Background(), nil, TaskInference, 1)
	if len(got) != 0 {
		t.Errorf("nil input: got %d, want 0", len(got))
	}
}

func TestSupportsTaskType_EmptyAcceptsAll(t *testing.T) {
	cases := []struct {
		supported []string
		task      TaskType
		want      bool
	}{
		{nil, TaskInference, true},
		{[]string{}, TaskFLRound, true},
		{[]string{"inference"}, TaskInference, true},
		{[]string{"inference"}, TaskFLRound, false},
		{[]string{"fl_round", "inference"}, TaskInference, true},
		{[]string{"gradient_agg"}, TaskAuditRepo, false},
	}
	for _, c := range cases {
		got := supportsTaskType(c.supported, c.task)
		if got != c.want {
			t.Errorf("supportsTaskType(%v, %q) = %v, want %v", c.supported, c.task, got, c.want)
		}
	}
}
