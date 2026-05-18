package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
)

// TestCanonicalRegisterMessageFormat verifies that signature inputs cannot be
// trivially confused by swapping fields (e.g. pubkey vs lnURI).
// This test is purely unit-level and requires no database.
func TestTierConstants(t *testing.T) {
	tiers := []string{registry.TierT1, registry.TierT2, registry.TierT3}
	want := []string{"t1", "t2", "t3"}
	for i, tier := range tiers {
		if tier != want[i] {
			t.Errorf("tier[%d] = %q, want %q", i, tier, want[i])
		}
	}
}

func TestStatusConstants(t *testing.T) {
	statuses := []string{
		registry.StatusPending,
		registry.StatusActive,
		registry.StatusDegraded,
		registry.StatusSuspended,
	}
	want := []string{"pending", "active", "degraded", "suspended"}
	for i, s := range statuses {
		if s != want[i] {
			t.Errorf("status[%d] = %q, want %q", i, s, want[i])
		}
	}
}

// TestNodeZeroValue ensures Node fields have safe zero values — no panics
// when accessing optional pointer fields.
func TestNodeZeroValue(t *testing.T) {
	var n registry.Node
	if n.LastHeartbeat != nil {
		t.Error("expected LastHeartbeat to be nil for zero-value Node")
	}
	if n.RegisteredAt.IsZero() {
		// This is expected for a zero-value struct.
		_ = n.RegisteredAt
	}
	_ = n.Reliability // float64 zero — should not panic
}

// TestNodeCapabilitiesZeroValue checks no panics on zero-value capabilities.
func TestNodeCapabilitiesZeroValue(t *testing.T) {
	caps := registry.NodeCapabilities{}
	if caps.Tier != "" {
		t.Errorf("expected empty tier for zero-value NodeCapabilities, got %q", caps.Tier)
	}
	if len(caps.SupportedTaskTypes) != 0 {
		t.Errorf("expected empty SupportedTaskTypes, got %v", caps.SupportedTaskTypes)
	}
}

// Integration tests require a live PostgreSQL instance.
// Run with: OWM_TEST_DSN=postgres://... go test ./internal/registry/...

func TestRegisterIntegration(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	reg := registry.New(pool, testutil.Logger())
	nf := testutil.NewNodeFixture(t)

	ts := time.Now().Unix()
	caps := registry.NodeCapabilities{
		Tier:               registry.TierT1,
		VRAMGB:             8,
		RAMGB:              16,
		BandwidthMbps:      100,
		SupportedTaskTypes: []string{"inference"},
	}
	sig := nf.Sign(nf.LNNodeURI, registry.TierT1, ts)

	node, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig, ts)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if node.Status != registry.StatusPending {
		t.Errorf("status: got %q, want %q", node.Status, registry.StatusPending)
	}
	if node.Tier != registry.TierT1 {
		t.Errorf("tier: got %q, want %q", node.Tier, registry.TierT1)
	}

	// Activate transitions pending → active.
	if err := reg.Activate(ctx, node.NodeID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := reg.GetByPublicKey(ctx, nf.PubKeyHex)
	if err != nil {
		t.Fatalf("GetByPublicKey: %v", err)
	}
	if got.Status != registry.StatusActive {
		t.Errorf("post-activate status: got %q, want %q", got.Status, registry.StatusActive)
	}

	// Re-registration resets to pending (idempotent upsert).
	ts2 := time.Now().Unix()
	sig2 := nf.Sign(nf.LNNodeURI, registry.TierT1, ts2)
	node2, err := reg.Register(ctx, nf.PubKeyHex, nf.LNNodeURI, "", caps, sig2, ts2)
	if err != nil {
		t.Fatalf("re-Register: %v", err)
	}
	if node2.NodeID != node.NodeID {
		t.Errorf("re-register: node_id changed: got %s, want %s", node2.NodeID, node.NodeID)
	}
	if node2.Status != registry.StatusPending {
		t.Errorf("re-register status: got %q, want %q", node2.Status, registry.StatusPending)
	}
}
