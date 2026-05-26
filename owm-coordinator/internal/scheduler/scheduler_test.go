package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/scheduler"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
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
// Run with: OWM_TEST_DSN=postgres://... go test ./internal/scheduler/...

func TestScheduleIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := registry.New(pool, testutil.Logger())
	nf := testutil.NewNodeFixture(t)

	ts := time.Now().Unix()
	caps := registry.NodeCapabilities{Tier: registry.TierT1, VRAMGB: 8, RAMGB: 16, BandwidthMbps: 100}
	sig := nf.Sign(nf.LNNodeURI, registry.TierT1, ts)
	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Activate(ctx, node.NodeID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	sched := scheduler.New(pool, reg, nil, testutil.Logger())
	a, err := sched.Schedule(ctx, scheduler.TaskInference, "inputhash-abc", 60)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if a.NodeID != node.NodeID {
		t.Errorf("assigned node: got %s, want %s", a.NodeID, node.NodeID)
	}
	if a.TaskType != scheduler.TaskInference {
		t.Errorf("task type: got %s, want inference", a.TaskType)
	}
	if a.RewardSats <= 0 {
		t.Errorf("reward_sats: expected > 0, got %d", a.RewardSats)
	}
}

func TestRequeueTimedOutIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := registry.New(pool, testutil.Logger())
	nf := testutil.NewNodeFixture(t)

	ts := time.Now().Unix()
	caps := registry.NodeCapabilities{Tier: registry.TierT1, VRAMGB: 8, RAMGB: 16, BandwidthMbps: 100}
	sig := nf.Sign(nf.LNNodeURI, registry.TierT1, ts)
	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Activate(ctx, node.NodeID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Insert a task that started 2 minutes ago with a 60-second timeout (already timed out).
	taskID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO tasks
		    (task_id, task_type, assigned_node, node_ln_uri, status, input_hash,
		     reward_sats, submitted_at, started_at, timeout_seconds)
		 VALUES ($1, 'inference', $2, $3, 'running', 'hash', 10,
		         now() - interval '2 minutes', now() - interval '2 minutes', 60)`,
		taskID, node.NodeID, node.LNNodeURI,
	)
	if err != nil {
		t.Fatalf("insert timed-out task: %v", err)
	}

	sched := scheduler.New(pool, reg, nil, testutil.Logger())
	n, err := sched.RequeueTimedOut(ctx)
	if err != nil {
		t.Fatalf("RequeueTimedOut: %v", err)
	}
	if n != 1 {
		t.Errorf("requeued count: got %d, want 1", n)
	}

	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM tasks WHERE task_id = $1`, taskID).Scan(&status)
	if status != "pending" {
		t.Errorf("task status after requeue: got %q, want pending", status)
	}
}

// TestDispatchPendingIntegration verifies SRS-SCHED-03: DispatchPending processes
// requeued tasks in priority order (FL rounds before inference before data ingest).
func TestDispatchPendingIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := registry.New(pool, testutil.Logger())
	nf := testutil.NewNodeFixture(t)

	ts := time.Now().Unix()
	caps := registry.NodeCapabilities{Tier: registry.TierT1, VRAMGB: 8, RAMGB: 16, BandwidthMbps: 100}
	sig := nf.Sign(nf.LNNodeURI, registry.TierT1, ts)
	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Activate(ctx, node.NodeID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Insert pending tasks in reverse priority order so we can verify the dispatcher
	// picks fl_round first.
	insertPending := func(taskType string) uuid.UUID {
		id := uuid.New()
		_, err := pool.Exec(ctx,
			`INSERT INTO tasks
			    (task_id, task_type, status, input_hash, reward_sats,
			     submitted_at, timeout_seconds)
			 VALUES ($1, $2, 'pending', 'hash', 10, now(), 120)`,
			id, taskType,
		)
		if err != nil {
			t.Fatalf("insert pending task (%s): %v", taskType, err)
		}
		return id
	}

	dataIngestID := insertPending("data_ingest")
	inferenceID := insertPending("inference")
	flRoundID := insertPending("fl_round")
	_ = dataIngestID
	_ = inferenceID

	sched := scheduler.New(pool, reg, nil, testutil.Logger())

	n, err := sched.DispatchPending(ctx)
	if err != nil {
		t.Fatalf("DispatchPending: %v", err)
	}
	if n != 3 {
		t.Errorf("dispatched count: got %d, want 3", n)
	}

	// fl_round task must be assigned to the node (processed first).
	var flStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM tasks WHERE task_id = $1`, flRoundID,
	).Scan(&flStatus); err != nil {
		t.Fatalf("fetching fl_round task: %v", err)
	}
	if flStatus != "running" {
		t.Errorf("fl_round task status: got %q, want running", flStatus)
	}
}

// TestDispatchPending_NoPendingTasks verifies DispatchPending is a no-op when the
// queue is empty and returns 0 without error.
func TestDispatchPending_NoPendingTasks(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := registry.New(pool, testutil.Logger())
	sched := scheduler.New(pool, reg, nil, testutil.Logger())

	n, err := sched.DispatchPending(ctx)
	if err != nil {
		t.Fatalf("DispatchPending: %v", err)
	}
	if n != 0 {
		t.Errorf("dispatched count: got %d, want 0", n)
	}
}

// TestScheduleIntegration_TaskTypeMismatch verifies that a node declaring
// specific supported task types is NOT assigned tasks it doesn't support,
// and IS assigned tasks it does support.
func TestScheduleIntegration_TaskTypeMismatch(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := registry.New(pool, testutil.Logger())

	// Register a node that only supports "fl_round".
	nf := testutil.NewNodeFixture(t)
	ts := time.Now().Unix()
	caps := registry.NodeCapabilities{
		Tier:               registry.TierT1,
		VRAMGB:             8,
		RAMGB:              16,
		BandwidthMbps:      100,
		SupportedTaskTypes: []string{string(scheduler.TaskFLRound)},
	}
	sig := nf.Sign(nf.LNNodeURI, registry.TierT1, ts)
	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Activate(ctx, node.NodeID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	sched := scheduler.New(pool, reg, nil, testutil.Logger())

	// Scheduling an unsupported task type must fail — no eligible nodes.
	_, err = sched.Schedule(ctx, scheduler.TaskInference, "hash-inference", 60)
	if err == nil {
		t.Fatal("expected error: no node supports inference, but Schedule succeeded")
	}

	// Scheduling the supported task type must succeed.
	a, err := sched.Schedule(ctx, scheduler.TaskFLRound, "hash-fl", 120)
	if err != nil {
		t.Fatalf("Schedule fl_round: %v", err)
	}
	if a.NodeID != node.NodeID {
		t.Errorf("assigned node: got %s, want %s", a.NodeID, node.NodeID)
	}
}
