package stake

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
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

// TestSlash_ZerosReliability verifies SRS-SEC-11: reliability is set to 0 atomically
// with status=suspended when a node is slashed.
func TestSlash_ZerosReliability(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()

	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status, reliability)
		 VALUES ($1, $2, $3, 't1', 'active', 0.9)`,
		nodeID, "pk-reliab-"+nodeID.String(), nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum, stake_status)
		 VALUES ($1, 'ch-reliab', 200000, 200000, 100000, 'active')`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	lnMock := mock.New()
	v := New(pool, lnMock, lnMock, SlashConfig{CooldownDuration: time.Hour}, zap.NewNop())

	if err := v.Slash(ctx, nodeID, "t1", "test", "evidence-hash", 3); err != nil {
		t.Fatalf("Slash: %v", err)
	}

	var reliability float64
	if err := pool.QueryRow(ctx,
		`SELECT reliability FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&reliability); err != nil {
		t.Fatalf("fetching reliability: %v", err)
	}
	if reliability != 0.0 {
		t.Errorf("reliability after slash: got %f, want 0.0", reliability)
	}
}

// TestVerifyAllActive_MarksNodeDegraded verifies SRS-STAKE-02: a node whose channel
// balance falls below tier minimum is marked degraded by the 6h re-verification pass.
func TestVerifyAllActive_MarksNodeDegraded(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	lnMock := mock.New()
	v := New(pool, lnMock, lnMock, SlashConfig{}, zap.NewNop())

	pubKey := "pk-verify-degrade-" + uuid.New().String()[:8]

	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'active')`,
		nodeID, pubKey, pubKey+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum, stake_status)
		 VALUES ($1, 'ch-degrade', 200000, 200000, 100000, 'active')`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	// Mock returns no channels → insufficient stake for any tier.
	lnMock.SetChannels(pubKey, []lightning.Channel{})

	if err := v.VerifyAllActive(ctx); err != nil {
		t.Fatalf("VerifyAllActive: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&status); err != nil {
		t.Fatalf("fetching node status: %v", err)
	}
	if status != "degraded" {
		t.Errorf("status: got %q, want degraded", status)
	}

	var degradedSince *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT degraded_since FROM node_stakes WHERE node_id = $1`, nodeID,
	).Scan(&degradedSince); err != nil {
		t.Fatalf("fetching degraded_since: %v", err)
	}
	if degradedSince == nil {
		t.Error("degraded_since not set in node_stakes")
	}
}

// TestVerifyAllActive_PassingNodeRefreshed verifies that a node with sufficient stake
// stays active and has its bonus_multiplier refreshed (SRS-STAKE-10).
func TestVerifyAllActive_PassingNodeRefreshed(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	lnMock := mock.New()
	v := New(pool, lnMock, lnMock, SlashConfig{}, zap.NewNop())

	pubKey := "pk-verify-pass-" + uuid.New().String()[:8]

	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'active')`,
		nodeID, pubKey, pubKey+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	// Initial stake at exactly tier minimum → bonus_multiplier = 1.0.
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum,
		     stake_status, bonus_multiplier)
		 VALUES ($1, 'ch-pass', 100000, 100000, 100000, 'active', 1.0)`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	// Channel capacity has grown to 200k (double the minimum) → bonus should become 1.5.
	// computeBonus(200k, 100k) = 1.0 + (100k/100k)*0.5 = 1.5
	lnMock.SetChannels(pubKey, []lightning.Channel{{
		ChannelID:        "ch-pass",
		RemotePubkey:     pubKey,
		CapacitySats:     200_000,
		LocalBalanceSats: 200_000,
		Active:           true,
	}})

	if err := v.VerifyAllActive(ctx); err != nil {
		t.Fatalf("VerifyAllActive: %v", err)
	}

	var nodeStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&nodeStatus); err != nil {
		t.Fatalf("fetching node status: %v", err)
	}
	if nodeStatus != "active" {
		t.Errorf("status: got %q, want active", nodeStatus)
	}

	var stakeStatus string
	var bonusMult float64
	if err := pool.QueryRow(ctx,
		`SELECT stake_status, bonus_multiplier FROM node_stakes WHERE node_id = $1`, nodeID,
	).Scan(&stakeStatus, &bonusMult); err != nil {
		t.Fatalf("fetching node_stakes: %v", err)
	}
	if stakeStatus != "active" {
		t.Errorf("stake_status: got %q, want active", stakeStatus)
	}
	// bonus_multiplier should have been updated to 1.5 (SRS-STAKE-10).
	const wantBonus = 1.5
	if bonusMult != wantBonus {
		t.Errorf("bonus_multiplier: got %f, want %f", bonusMult, wantBonus)
	}
}

// TestEnforceDegradedGrace_SuspendsExpired verifies SRS-STAKE-03: a node that has
// been degraded beyond the grace period is automatically suspended.
func TestEnforceDegradedGrace_SuspendsExpired(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	v := New(pool, nil, nil, SlashConfig{}, zap.NewNop())

	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'degraded')`,
		nodeID, "pk-grace-expired-"+nodeID.String()[:8], nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	// degraded_since 25 hours ago → past the 24h grace period.
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum,
		     stake_status, degraded_since)
		 VALUES ($1, 'ch-expired', 200000, 50000, 100000, 'degraded', now() - interval '25 hours')`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	if err := v.EnforceDegradedGrace(ctx, 24*time.Hour); err != nil {
		t.Fatalf("EnforceDegradedGrace: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&status); err != nil {
		t.Fatalf("fetching node status: %v", err)
	}
	if status != "suspended" {
		t.Errorf("status: got %q, want suspended", status)
	}
}

// TestEnforceDegradedGrace_SkipsFreshDegraded verifies that a recently-degraded node
// is not suspended before the grace period expires.
func TestEnforceDegradedGrace_SkipsFreshDegraded(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	v := New(pool, nil, nil, SlashConfig{}, zap.NewNop())

	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'degraded')`,
		nodeID, "pk-grace-fresh-"+nodeID.String()[:8], nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	// degraded_since only 1 hour ago → still within the 24h grace period.
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum,
		     stake_status, degraded_since)
		 VALUES ($1, 'ch-fresh', 200000, 50000, 100000, 'degraded', now() - interval '1 hour')`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	if err := v.EnforceDegradedGrace(ctx, 24*time.Hour); err != nil {
		t.Fatalf("EnforceDegradedGrace: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&status); err != nil {
		t.Fatalf("fetching node status: %v", err)
	}
	if status != "degraded" {
		t.Errorf("status: got %q, want degraded (should not be suspended yet)", status)
	}
}

// insertSlashedNode is a helper that inserts a node, its stake record, and a
// slashing event with a configurable cooldown_expires_at, then returns the node_id
// and public_key.
func insertSlashedNode(t *testing.T, pool *pgxpool.Pool, cooldownExpires time.Time) (uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	nodeID := uuid.New()
	pubKey := "pk-cooldown-" + nodeID.String()[:8]

	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'suspended')`,
		nodeID, pubKey, pubKey+"@127.0.0.1:9735",
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum, stake_status)
		 VALUES ($1, 'ch-cooldown', 200000, 0, 100000, 'force_closed')`,
		nodeID,
	); err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO slashing_events
		    (node_id, channel_id, tier, reason, evidence_hash, signal_count, cooldown_expires_at, coordinator_sig)
		 VALUES ($1, 'ch-cooldown', 't1', 'test', 'evidence', 3, $2, 'pending')`,
		nodeID, cooldownExpires,
	); err != nil {
		t.Fatalf("insert slashing_event: %v", err)
	}
	return nodeID, pubKey
}

// TestCheckCooldown_NotSlashed verifies a never-slashed node returns nil.
func TestCheckCooldown_NotSlashed(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	nodeID := uuid.New()
	pubKey := "pk-no-slash-" + nodeID.String()[:8]
	_, err := pool.Exec(context.Background(),
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'active')`,
		nodeID, pubKey, pubKey+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	v := New(pool, nil, nil, SlashConfig{}, zap.NewNop())
	if err := v.CheckCooldown(context.Background(), pubKey); err != nil {
		t.Errorf("expected nil for non-slashed node, got: %v", err)
	}
}

// TestCheckCooldown_ActiveCooldown verifies a recently-slashed node is blocked.
func TestCheckCooldown_ActiveCooldown(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	_, pubKey := insertSlashedNode(t, pool, time.Now().Add(30*24*time.Hour))
	v := New(pool, nil, nil, SlashConfig{}, zap.NewNop())
	err := v.CheckCooldown(context.Background(), pubKey)
	if err == nil {
		t.Fatal("expected error for node in active cooldown, got nil")
	}
	if !strings.Contains(err.Error(), "STAKE_COOLDOWN") {
		t.Errorf("expected STAKE_COOLDOWN in error, got: %v", err)
	}
}

// TestCheckCooldown_ExpiredCooldown verifies a node whose cooldown has passed is allowed.
func TestCheckCooldown_ExpiredCooldown(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	_, pubKey := insertSlashedNode(t, pool, time.Now().Add(-time.Hour))
	v := New(pool, nil, nil, SlashConfig{}, zap.NewNop())
	if err := v.CheckCooldown(context.Background(), pubKey); err != nil {
		t.Errorf("expected nil for expired cooldown, got: %v", err)
	}
}

// TestRecordMaintainerAck_BelowThreshold verifies a single ack for a T2 node
// (threshold=2) records but does not trigger slashing.
func TestRecordMaintainerAck_BelowThreshold(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	nodeID := uuid.New()
	pubKey := "pk-ack-t2-" + nodeID.String()[:8]

	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't2', 'active')`,
		nodeID, pubKey, pubKey+"@127.0.0.1:9735",
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	lnMock := mock.New()
	cfg := SlashConfig{T2T3MaintainerAcks: 2, CooldownDuration: 30 * 24 * time.Hour}
	v := New(pool, lnMock, lnMock, cfg, zap.NewNop())

	maintPub, maintPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	reasonHash := "deadbeef"
	msg := []byte("owm-slash-ack|" + nodeID.String() + "|" + reasonHash)
	sig := ed25519.Sign(maintPriv, msg)

	if err := v.RecordMaintainerAck(ctx, nodeID, "t2",
		hex.EncodeToString(maintPub), hex.EncodeToString(sig), reasonHash); err != nil {
		t.Fatalf("RecordMaintainerAck: %v", err)
	}

	// Node should still be active — threshold not yet reached.
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&status); err != nil {
		t.Fatalf("fetching status: %v", err)
	}
	if status != "active" {
		t.Errorf("status: got %q, want active (ack below threshold)", status)
	}
}

// TestRecordMaintainerAck_TriggersSlash verifies that hitting the ack threshold
// automatically triggers slashing (SRS-SEC-14).
func TestRecordMaintainerAck_TriggersSlash(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	nodeID := uuid.New()
	pubKey := "pk-ack-slash-" + nodeID.String()[:8]

	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't2', 'active')`,
		nodeID, pubKey, pubKey+"@127.0.0.1:9735",
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum, stake_status)
		 VALUES ($1, 'ch-ack-slash', 600000, 600000, 500000, 'active')`,
		nodeID,
	); err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	lnMock := mock.New()
	cfg := SlashConfig{T2T3MaintainerAcks: 2, CooldownDuration: 30 * 24 * time.Hour}
	v := New(pool, lnMock, lnMock, cfg, zap.NewNop())

	reasonHash := "cafebabe"

	sendAck := func(t *testing.T) {
		t.Helper()
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		msg := []byte("owm-slash-ack|" + nodeID.String() + "|" + reasonHash)
		sig := ed25519.Sign(priv, msg)
		if err := v.RecordMaintainerAck(ctx, nodeID, "t2",
			hex.EncodeToString(pub), hex.EncodeToString(sig), reasonHash); err != nil {
			t.Fatalf("RecordMaintainerAck: %v", err)
		}
	}

	sendAck(t) // ack 1 — below threshold
	sendAck(t) // ack 2 — hits threshold, triggers slash

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM nodes WHERE node_id = $1`, nodeID,
	).Scan(&status); err != nil {
		t.Fatalf("fetching status: %v", err)
	}
	if status != "suspended" {
		t.Errorf("status: got %q, want suspended (slash should have been triggered)", status)
	}

	var slashCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM slashing_events WHERE node_id = $1`, nodeID,
	).Scan(&slashCount); err != nil {
		t.Fatalf("fetching slash events: %v", err)
	}
	if slashCount != 1 {
		t.Errorf("slashing_events: got %d, want 1", slashCount)
	}
}

// TestRecordMaintainerAck_InvalidSignature verifies that a bad signature is rejected.
func TestRecordMaintainerAck_InvalidSignature(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	nodeID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't2', 'active')`,
		nodeID, "pk-badsig-"+nodeID.String()[:8], nodeID.String()+"@127.0.0.1:9735",
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	lnMock := mock.New()
	v := New(pool, lnMock, lnMock, SlashConfig{T2T3MaintainerAcks: 2}, zap.NewNop())

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	badSig := make([]byte, ed25519.SignatureSize)

	err = v.RecordMaintainerAck(ctx, nodeID, "t2",
		hex.EncodeToString(pub), hex.EncodeToString(badSig), "deadbeef")
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}
