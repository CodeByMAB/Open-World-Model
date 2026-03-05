package stake_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

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

// TestVerifyStake_NilClient asserts that when lnReadonly is nil (dev mode),
// VerifyStake returns a synthetic OK result without calling Lightning.
func TestVerifyStake_NilClient(t *testing.T) {
	dsn := os.Getenv("OWM_TEST_DSN")
	if dsn == "" {
		t.Skip("OWM_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to test DB: %v", err)
	}
	defer pool.Close()
	log := zap.NewNop()
	v := stake.New(pool, nil, nil, stake.SlashConfig{
		T1AutoSlashSignals: 3,
		T2T3MaintainerAcks: 2,
		CooldownDuration:   30 * 24 * time.Hour,
	}, log)
	result, err := v.VerifyStake(ctx, "any-pubkey", "t1")
	if err != nil {
		t.Fatalf("VerifyStake with nil client: %v", err)
	}
	if !result.OK {
		t.Errorf("expected synthetic OK when lnReadonly is nil, got OK=false")
	}
	if result.ChannelID != "dev-no-ln-client" {
		t.Errorf("expected ChannelID dev-no-ln-client, got %q", result.ChannelID)
	}
}

// Integration tests require a live LND node and PostgreSQL instance.
func TestVerifyStakeIntegration(t *testing.T) {
	t.Skip("integration test — requires live LND node and OWM_TEST_DSN")
}

func TestSlashIntegration(t *testing.T) {
	t.Skip("integration test — requires live LND node and OWM_TEST_DSN")
}
