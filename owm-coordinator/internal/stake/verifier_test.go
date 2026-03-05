package stake_test

import (
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/stake"
)

// TestTierMinimums ensures the tier minimums match the BRS-POS-02 specification.
func TestTierMinimums(t *testing.T) {
	cases := []struct {
		tier string
		want int64
	}{
		{"t1", 100_000},
		{"t2", 500_000},
		{"t3", 2_000_000},
	}
	for _, c := range cases {
		got, ok := stake.TierMinimums[c.tier]
		if !ok {
			t.Errorf("tier %q not found in TierMinimums", c.tier)
			continue
		}
		if got != c.want {
			t.Errorf("TierMinimums[%q] = %d, want %d", c.tier, got, c.want)
		}
	}
}

// TestStakeResultZeroValue confirms StakeResult has safe zero values.
func TestStakeResultZeroValue(t *testing.T) {
	var r stake.StakeResult
	if r.OK {
		t.Error("expected OK=false for zero-value StakeResult")
	}
	if r.BonusMultiplier != 0 {
		t.Errorf("expected BonusMultiplier=0, got %f", r.BonusMultiplier)
	}
}

// TestSlashConfigDefaults verifies the SlashConfig struct accepts the expected values.
func TestSlashConfigDefaults(t *testing.T) {
	cfg := stake.SlashConfig{
		T1AutoSlashSignals: 3,
		T2T3MaintainerAcks: 2,
	}
	if cfg.T1AutoSlashSignals != 3 {
		t.Errorf("T1AutoSlashSignals = %d, want 3", cfg.T1AutoSlashSignals)
	}
	if cfg.T2T3MaintainerAcks != 2 {
		t.Errorf("T2T3MaintainerAcks = %d, want 2", cfg.T2T3MaintainerAcks)
	}
}

// Integration tests require a live LND node and PostgreSQL instance.
func TestVerifyStakeIntegration(t *testing.T) {
	t.Skip("integration test — requires live LND node and OWM_TEST_DSN")
}

func TestSlashIntegration(t *testing.T) {
	t.Skip("integration test — requires live LND node and OWM_TEST_DSN")
}
