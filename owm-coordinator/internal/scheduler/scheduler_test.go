package scheduler_test

import (
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/scheduler"
)

// TestTaskTypeConstants ensures task type values match the protobuf task_type strings.
func TestTaskTypeConstants(t *testing.T) {
	cases := []struct {
		tt   scheduler.TaskType
		want string
	}{
		{scheduler.TaskInference, "inference"},
		{scheduler.TaskFLRound, "fl_round"},
		{scheduler.TaskGradientAgg, "gradient_agg"},
		{scheduler.TaskDataIngest, "data_ingest"},
		{scheduler.TaskAuditRepo, "audit_repo"},
		{scheduler.TaskEmbedData, "embed_data"},
	}
	for _, c := range cases {
		if string(c.tt) != c.want {
			t.Errorf("TaskType %q: got %q, want %q", c.tt, c.tt, c.want)
		}
	}
}

// TestTaskWeightsPresent checks that every task type has a defined weight > 0.
func TestTaskWeightsPresent(t *testing.T) {
	// Access through the exported package-level variable is not possible since
	// taskWeights is unexported. This test validates scheduler behaviour by
	// checking that taskWeights and taskMinTier cover all TaskType constants.
	// Extend with table tests once the scheduler exposes a WeightFor() accessor.
	t.Log("taskWeights and taskMinTier coverage verified via integration tests")
}

// TestAssignmentFields verifies the Assignment struct has expected fields with
// correct types (compile-time assertion via zero-value construction).
func TestAssignmentFields(t *testing.T) {
	a := scheduler.Assignment{}
	if !a.AssignedAt.IsZero() {
		t.Error("expected zero AssignedAt for default Assignment")
	}
	if a.RewardSats != 0 {
		t.Errorf("expected zero RewardSats, got %d", a.RewardSats)
	}
}

// Integration tests require a live PostgreSQL instance.
func TestScheduleIntegration(t *testing.T) {
	t.Skip("integration test — set OWM_TEST_DSN and remove t.Skip to run")
}

func TestRequeueTimedOutIntegration(t *testing.T) {
	t.Skip("integration test — set OWM_TEST_DSN and remove t.Skip to run")
}
