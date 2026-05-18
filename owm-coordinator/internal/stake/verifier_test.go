package stake

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/lightning/mock"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
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

// TestSlash_MockForceClose is an integration test: it requires a live PostgreSQL
// instance but uses a mock Lightning client so no real LN node is needed.
// Run with: OWM_TEST_DSN=postgres://... go test ./internal/stake/...
func TestSlash_MockForceClose(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()

	// Insert a node and its stake record directly; no need for the registry.
	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'active')`,
		nodeID, "pk-slash-"+nodeID.String(), nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum, stake_status)
		 VALUES ($1, 'test-channel-slash', 200000, 200000, 100000, 'active')`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	lnMock := mock.New()
	v := New(pool, lnMock, lnMock, SlashConfig{CooldownDuration: time.Hour}, zap.NewNop())

	if err := v.Slash(ctx, nodeID, "t1", "test-misbehavior", "evidence-hash-abc", 3); err != nil {
		t.Fatalf("Slash: %v", err)
	}

	// Slashing event must be recorded.
	var eventCount int
	_ = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM slashing_events WHERE node_id = $1`, nodeID,
	).Scan(&eventCount)
	if eventCount != 1 {
		t.Errorf("slashing_events: got %d, want 1", eventCount)
	}

	// Node must be suspended.
	var status string
	_ = pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&status)
	if status != "suspended" {
		t.Errorf("node status after slash: got %q, want suspended", status)
	}

	// Stake must be marked force_closed.
	var stakeStatus string
	_ = pool.QueryRow(ctx,
		`SELECT stake_status FROM node_stakes WHERE node_id = $1`, nodeID,
	).Scan(&stakeStatus)
	if stakeStatus != "force_closed" {
		t.Errorf("stake_status: got %q, want force_closed", stakeStatus)
	}
}
