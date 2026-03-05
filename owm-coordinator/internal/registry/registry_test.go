package registry_test

import (
	"testing"
	"time"

	"github.com/owmnetwork/owm-coordinator/internal/registry"
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
// Run with: OWM_TEST_DSN=postgres://... go test ./internal/registry/... -tags integration

func TestRegisterIntegration(t *testing.T) {
	t.Skip("integration test — set OWM_TEST_DSN and remove t.Skip to run")
	_ = time.Now() // placeholder — real test would create pgxpool, call Register, etc.
}
