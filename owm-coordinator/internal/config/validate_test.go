package config

import (
	"strings"
	"testing"
)

// validBase returns a minimal Config that passes every validate() check.
// Individual tests mutate one field at a time to trigger a specific error.
func validBase() *Config {
	return &Config{
		DevMode:  true, // skip Lightning credential checks
		Database: DatabaseConfig{DSN: "postgres://localhost/test"},
		Lightning: LightningConfig{Backend: "lnd"},
		FL: FLConfig{
			MinParticipants:        2,
			GradientL2ClipNorm:     1.0,
			AnomalyStdDevThreshold: 3.0,
			RoundIntervalMinutes:   1,
			TopKSparsificationPct:  0.10,
		},
		Stake: StakeConfig{
			TierMinimumSats: map[string]int64{
				"t1": 100_000, "t2": 500_000, "t3": 2_000_000,
			},
			VerifyIntervalHours:      1,
			DegradedGracePeriodHours: 1,
			SlashCooldownDays:        1,
			T1AutoSlashSignals:       3,
			T2T3MaintainerAcks:       2,
		},
	}
}

func mustFail(t *testing.T, cfg *Config, wantSubstr string) {
	t.Helper()
	err := cfg.validate()
	if err == nil {
		t.Fatalf("expected validation error containing %q, got nil", wantSubstr)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

func mustPass(t *testing.T, cfg *Config) {
	t.Helper()
	if err := cfg.validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

// ── Tier minimums ─────────────────────────────────────────────────────────────

func TestValidate_TierFloor_T1(t *testing.T) {
	cfg := validBase()
	cfg.Stake.TierMinimumSats["t1"] = 99_999
	mustFail(t, cfg, "tier_minimum_sats.t1")
}

func TestValidate_TierFloor_T2(t *testing.T) {
	cfg := validBase()
	cfg.Stake.TierMinimumSats["t2"] = 499_999
	mustFail(t, cfg, "tier_minimum_sats.t2")
}

func TestValidate_TierFloor_T3(t *testing.T) {
	cfg := validBase()
	cfg.Stake.TierMinimumSats["t3"] = 1_999_999
	mustFail(t, cfg, "tier_minimum_sats.t3")
}

func TestValidate_TierFloor_NilMap(t *testing.T) {
	cfg := validBase()
	cfg.Stake.TierMinimumSats = nil // all tiers read as 0 → below every floor
	mustFail(t, cfg, "tier_minimum_sats.t1")
}

func TestValidate_TierOrdering_T1GtT2(t *testing.T) {
	cfg := validBase()
	cfg.Stake.TierMinimumSats["t1"] = 600_000 // above t2
	mustFail(t, cfg, "t1 ≤ t2 ≤ t3")
}

func TestValidate_TierOrdering_T2GtT3(t *testing.T) {
	cfg := validBase()
	cfg.Stake.TierMinimumSats["t2"] = 3_000_000 // above t3
	mustFail(t, cfg, "t1 ≤ t2 ≤ t3")
}

func TestValidate_TierOrdering_EqualBoundariesOK(t *testing.T) {
	// t1 == t2 == t3 is allowed by the ordering check (≤ not <).
	cfg := validBase()
	cfg.Stake.TierMinimumSats["t1"] = 2_000_000
	cfg.Stake.TierMinimumSats["t2"] = 2_000_000
	cfg.Stake.TierMinimumSats["t3"] = 2_000_000
	mustPass(t, cfg)
}

// ── Stake operational config ──────────────────────────────────────────────────

func TestValidate_Stake_VerifyIntervalZero(t *testing.T) {
	cfg := validBase()
	cfg.Stake.VerifyIntervalHours = 0
	mustFail(t, cfg, "verify_interval_hours")
}

func TestValidate_Stake_DegradedGracePeriodZero(t *testing.T) {
	cfg := validBase()
	cfg.Stake.DegradedGracePeriodHours = 0
	mustFail(t, cfg, "degraded_grace_period_hours")
}

func TestValidate_Stake_SlashCooldownZero(t *testing.T) {
	cfg := validBase()
	cfg.Stake.SlashCooldownDays = 0
	mustFail(t, cfg, "slash_cooldown_days")
}

func TestValidate_Stake_T1AutoSlashSignalsTooLow(t *testing.T) {
	cfg := validBase()
	cfg.Stake.T1AutoSlashSignals = 2
	mustFail(t, cfg, "t1_auto_slash_signals")
}

func TestValidate_Stake_T2T3MaintainerAcksTooLow(t *testing.T) {
	cfg := validBase()
	cfg.Stake.T2T3MaintainerAcks = 1
	mustFail(t, cfg, "t2t3_maintainer_acks")
}

func TestValidate_Stake_HigherThresholdsOK(t *testing.T) {
	cfg := validBase()
	cfg.Stake.T1AutoSlashSignals = 10 // more conservative than SRS minimum
	cfg.Stake.T2T3MaintainerAcks = 5
	mustPass(t, cfg)
}

// ── FL config bounds ──────────────────────────────────────────────────────────

func TestValidate_FL_MinParticipantsTooLow(t *testing.T) {
	cfg := validBase()
	cfg.FL.MinParticipants = 1
	mustFail(t, cfg, "min_participants")
}

func TestValidate_FL_MinParticipantsExactly2(t *testing.T) {
	cfg := validBase()
	cfg.FL.MinParticipants = 2
	mustPass(t, cfg)
}

func TestValidate_FL_GradientClipNormZero(t *testing.T) {
	cfg := validBase()
	cfg.FL.GradientL2ClipNorm = 0
	mustFail(t, cfg, "gradient_l2_clip_norm")
}

func TestValidate_FL_GradientClipNormNegative(t *testing.T) {
	cfg := validBase()
	cfg.FL.GradientL2ClipNorm = -1
	mustFail(t, cfg, "gradient_l2_clip_norm")
}

func TestValidate_FL_AnomalyThresholdZero(t *testing.T) {
	cfg := validBase()
	cfg.FL.AnomalyStdDevThreshold = 0
	mustFail(t, cfg, "anomaly_std_dev_threshold")
}

func TestValidate_FL_RoundIntervalZero(t *testing.T) {
	cfg := validBase()
	cfg.FL.RoundIntervalMinutes = 0
	mustFail(t, cfg, "round_interval_minutes")
}

func TestValidate_FL_TopKPctNegative(t *testing.T) {
	cfg := validBase()
	cfg.FL.TopKSparsificationPct = -0.01
	mustFail(t, cfg, "top_k_sparsification_pct")
}

func TestValidate_FL_TopKPctAbove1(t *testing.T) {
	cfg := validBase()
	cfg.FL.TopKSparsificationPct = 1.01
	mustFail(t, cfg, "top_k_sparsification_pct")
}

func TestValidate_FL_TopKPctZeroOK(t *testing.T) {
	cfg := validBase()
	cfg.FL.TopKSparsificationPct = 0 // 0 = sparsification disabled
	mustPass(t, cfg)
}

func TestValidate_FL_TopKPctOneOK(t *testing.T) {
	cfg := validBase()
	cfg.FL.TopKSparsificationPct = 1.0 // keep all
	mustPass(t, cfg)
}

// ── S3 group check ────────────────────────────────────────────────────────────

func TestValidate_S3_AllEmptyOK(t *testing.T) {
	cfg := validBase() // S3 fields are all zero-value → OK
	mustPass(t, cfg)
}

func TestValidate_S3_AllSetOK(t *testing.T) {
	cfg := validBase()
	cfg.S3 = S3Config{
		Endpoint:  "https://s3.example.com",
		Bucket:    "owm-data",
		AccessKey: "AKID",
		SecretKey: "secret",
	}
	mustPass(t, cfg)
}

func TestValidate_S3_PartialMissingBucket(t *testing.T) {
	cfg := validBase()
	cfg.S3 = S3Config{Endpoint: "https://s3.example.com", AccessKey: "AKID", SecretKey: "secret"}
	mustFail(t, cfg, "s3 is partially configured")
}

func TestValidate_S3_PartialOnlyEndpoint(t *testing.T) {
	cfg := validBase()
	cfg.S3 = S3Config{Endpoint: "https://s3.example.com"}
	mustFail(t, cfg, "s3 is partially configured")
}

func TestValidate_S3_PartialOnlyBucket(t *testing.T) {
	cfg := validBase()
	cfg.S3 = S3Config{Bucket: "owm-data"}
	mustFail(t, cfg, "s3 is partially configured")
}

// ── Full valid config ─────────────────────────────────────────────────────────

func TestValidate_BaseConfigPasses(t *testing.T) {
	mustPass(t, validBase())
}
