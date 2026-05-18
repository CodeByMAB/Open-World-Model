package registry

// computeReliability is unexported; these tests live in package registry.

import (
	"context"
	"testing"
	"time"

	"github.com/owmnetwork/owm-coordinator/internal/testutil"
)

// ── computeReliability unit tests ─────────────────────────────────────────────

func TestComputeReliability_NoTasksNoHeartbeats_NewNode(t *testing.T) {
	// Brand-new node: window < 1 heartbeat interval → uptime defaults to 1.0.
	// No tasks → task fraction defaults to 1.0. Result: 1.0.
	got := computeReliability(0, 0, 0, 30*time.Second)
	if got != 1.0 {
		t.Errorf("got %f, want 1.0", got)
	}
}

func TestComputeReliability_PerfectRecord(t *testing.T) {
	// 10/10 tasks completed, heartbeat every 60 s for 1 hour (60 received, 60 expected).
	got := computeReliability(10, 10, 60, time.Hour)
	if got != 1.0 {
		t.Errorf("got %f, want 1.0", got)
	}
}

func TestComputeReliability_HalfUptimeFullSuccess(t *testing.T) {
	// All tasks succeeded but node was online only 50 % of the time.
	// 1 hour window → 60 expected heartbeats; only 30 received.
	got := computeReliability(5, 5, 30, time.Hour)
	const want = 0.5
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestComputeReliability_HalfSuccessFullUptime(t *testing.T) {
	// Node was always online but only completed half its tasks.
	got := computeReliability(5, 10, 60, time.Hour)
	const want = 0.5
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestComputeReliability_HalfUptimeHalfSuccess(t *testing.T) {
	// 0.5 * 0.5 = 0.25
	got := computeReliability(5, 10, 30, time.Hour)
	const want = 0.25
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestComputeReliability_ZeroTasksWithHeartbeats(t *testing.T) {
	// No tasks yet → task fraction = 1.0; uptime is real.
	got := computeReliability(0, 0, 30, time.Hour) // 50 % uptime
	const want = 0.5
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func TestComputeReliability_NoHeartbeatsLongWindow(t *testing.T) {
	// Node has been around for a day but sent zero heartbeats → uptime = 0.
	got := computeReliability(10, 10, 0, 24*time.Hour)
	if got != 0.0 {
		t.Errorf("got %f, want 0.0", got)
	}
}

func TestComputeReliability_MoreHeartbeatsThanExpected(t *testing.T) {
	// Extra heartbeats (e.g. from retries) must not push reliability above 1.0.
	got := computeReliability(10, 10, 200, time.Hour) // expected 60, received 200
	if got != 1.0 {
		t.Errorf("got %f, want 1.0 (capped)", got)
	}
}

func TestComputeReliability_WindowExactlyOneInterval(t *testing.T) {
	// Edge: exactly 60 s window → expectedHB = 1.0.
	// 1 heartbeat received → uptime = 1.0; 1/1 tasks completed → 1.0.
	got := computeReliability(1, 1, 1, 60*time.Second)
	if got != 1.0 {
		t.Errorf("got %f, want 1.0", got)
	}
}

func TestComputeReliability_WindowSlightlyBelowOneInterval(t *testing.T) {
	// < 60 s window → expectedHB < 1 → uptime defaults to 1.0.
	got := computeReliability(0, 0, 0, 59*time.Second)
	if got != 1.0 {
		t.Errorf("got %f, want 1.0 (window too small, defaults)", got)
	}
}

// ── Integration: UpdateReliability with real DB ───────────────────────────────

func TestUpdateReliabilityIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := New(pool, testutil.Logger())
	nf := testutil.NewNodeFixture(t)

	// Register and activate the node.
	ts := time.Now().Unix()
	caps := NodeCapabilities{Tier: TierT1, VRAMGB: 8, RAMGB: 16, BandwidthMbps: 100}
	sig := nf.Sign(nf.LNNodeURI, TierT1, ts)
	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Activate(ctx, node.NodeID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Backdate registered_at so the window start (max(now-7d, registered_at))
	// is 2 hours ago, allowing the historical heartbeat and task data below to
	// fall inside the window.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET registered_at = now() - interval '2 hours' WHERE node_id = $1`,
		node.NodeID,
	); err != nil {
		t.Fatalf("backdate registered_at: %v", err)
	}

	// Seed 60 heartbeats directly (representing ~1 hour of uptime).
	base := time.Now().UTC().Add(-61 * time.Minute)
	for i := 0; i < 60; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO heartbeat_log (node_id, recorded_at) VALUES ($1, $2)`,
			node.NodeID, base.Add(time.Duration(i)*time.Minute),
		)
		if err != nil {
			t.Fatalf("insert heartbeat %d: %v", i, err)
		}
	}

	// Seed 8 tasks: 6 completed, 2 failed — task_fraction = 0.75.
	insertTask := func(status string) {
		_, err := pool.Exec(ctx,
			`INSERT INTO tasks
			    (task_id, task_type, assigned_node, node_ln_uri, status,
			     input_hash, reward_sats, submitted_at, started_at, timeout_seconds)
			 VALUES (gen_random_uuid(), 'inference', $1, $2, $3,
			         'hash', 10, now() - interval '30 minutes', now() - interval '30 minutes', 300)`,
			node.NodeID, node.LNNodeURI, status,
		)
		if err != nil {
			t.Fatalf("insert task (%s): %v", status, err)
		}
	}
	for i := 0; i < 6; i++ {
		insertTask("completed")
	}
	for i := 0; i < 2; i++ {
		insertTask("failed")
	}

	// UpdateReliability with success=true for the last task.
	if err := reg.UpdateReliability(ctx, node.NodeID, true); err != nil {
		t.Fatalf("UpdateReliability: %v", err)
	}

	// Fetch the persisted reliability score.
	var reliability float64
	if err := pool.QueryRow(ctx,
		`SELECT reliability FROM nodes WHERE node_id = $1`, node.NodeID,
	).Scan(&reliability); err != nil {
		t.Fatalf("fetching reliability: %v", err)
	}

	// task_fraction ≈ 6/8 = 0.75; uptime_fraction ≈ 60/61 ≈ 0.98 → reliability ≈ 0.735
	// We use a tolerance of ±0.05 to account for timing jitter.
	const wantApprox = 0.75
	const tolerance = 0.05
	if reliability < wantApprox-tolerance || reliability > wantApprox+tolerance {
		t.Errorf("reliability = %f, want %f ± %f", reliability, wantApprox, tolerance)
	}
	if reliability <= 0 || reliability > 1 {
		t.Errorf("reliability %f out of [0,1] range", reliability)
	}
}

func TestRecordHeartbeatIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := New(pool, testutil.Logger())
	nf := testutil.NewNodeFixture(t)

	ts := time.Now().Unix()
	caps := NodeCapabilities{Tier: TierT1, VRAMGB: 8, RAMGB: 16, BandwidthMbps: 100}
	sig := nf.Sign(nf.LNNodeURI, TierT1, ts)
	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err = reg.RecordHeartbeat(ctx, node.NodeID)
	if err != nil {
		t.Fatalf("RecordHeartbeat: %v", err)
	}

	// Verify one row was inserted into heartbeat_log.
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM heartbeat_log WHERE node_id = $1`, node.NodeID,
	).Scan(&count); err != nil {
		t.Fatalf("count heartbeat_log: %v", err)
	}
	if count != 1 {
		t.Errorf("heartbeat_log rows: got %d, want 1", count)
	}

	// Verify last_heartbeat was updated in the nodes table.
	var lastHB *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT last_heartbeat FROM nodes WHERE node_id = $1`, node.NodeID,
	).Scan(&lastHB); err != nil {
		t.Fatalf("fetch last_heartbeat: %v", err)
	}
	if lastHB == nil {
		t.Error("last_heartbeat was not updated in nodes table")
	}
}
