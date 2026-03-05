package stake

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/lightning/mock"
)

func TestVerifyStake_SufficientStake(t *testing.T) {
	ctx := context.Background()
	// Use nil db for unit test (verifier only needs LN for this path).
	var db *pgxpool.Pool
	log := zap.NewNop()
	client := mock.New()
	client.SetChannels("pubkey1", []lightning.Channel{{
		ChannelID:        "ch1",
		RemotePubkey:     "pubkey1",
		CapacitySats:     200_000,
		LocalBalanceSats: 200_000,
		Active:           true,
	}})
	v := New(db, client, client, SlashConfig{}, log)
	result, err := v.VerifyStake(ctx, "pubkey1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Errorf("expected OK: %+v", result)
	}
	if result.CapacitySats != 200_000 {
		t.Errorf("CapacitySats: got %d", result.CapacitySats)
	}
}

func TestVerifyStake_InsufficientStake(t *testing.T) {
	ctx := context.Background()
	var db *pgxpool.Pool
	log := zap.NewNop()
	client := mock.New()
	// t2 needs 500_000; give 100_000
	client.SetChannels("pubkey2", []lightning.Channel{{
		ChannelID:        "ch2",
		RemotePubkey:     "pubkey2",
		CapacitySats:     100_000,
		LocalBalanceSats: 100_000,
		Active:           true,
	}})
	v := New(db, client, client, SlashConfig{}, log)
	result, err := v.VerifyStake(ctx, "pubkey2", "t2")
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected !OK for insufficient stake")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error")
	}
}

func TestVerifyStake_NilClient(t *testing.T) {
	ctx := context.Background()
	var db *pgxpool.Pool
	log := zap.NewNop()
	v := New(db, nil, nil, SlashConfig{}, log)
	result, err := v.VerifyStake(ctx, "pubkey", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("nil client: expected OK false")
	}
	if result.Error != "LN client not configured (dev_mode?)" {
		t.Errorf("nil client: expected config error, got %q", result.Error)
	}
}

func TestSlash_NilClient(t *testing.T) {
	ctx := context.Background()
	var db *pgxpool.Pool
	log := zap.NewNop()
	v := New(db, nil, nil, SlashConfig{}, log)
	err := v.Slash(ctx, uuid.Nil, "t1", "reason", "hash", 1)
	if err == nil {
		t.Error("expected error when lnSlash is nil")
	}
}

// TestSlash_MockForceClose would require DB (node_stakes, slashing_events, etc.).
// Plan: "DB interaction must be mocked with pgxmock or skipped via build tag".
// We skip it here; integration tests can cover full Slash with mock LN.
func TestSlash_MockForceClose(t *testing.T) {
	t.Skip("Slash requires DB; use integration test or pgxmock")
}
