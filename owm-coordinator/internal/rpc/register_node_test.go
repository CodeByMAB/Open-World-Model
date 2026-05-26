package rpc_test

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/lightning/mock"
	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/rpc"
	"github.com/owmnetwork/owm-coordinator/internal/stake"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
	coordinatorv1 "github.com/owmnetwork/owm-coordinator/proto/coordinator/v1"
)

// buildServer constructs a minimal rpc.Server for integration tests.
// scheduler, fl, disp, rdb are nil because RegisterNode does not use them.
func buildServer(t *testing.T, lnMock *mock.Client) (*rpc.Server, func()) {
	t.Helper()
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	reg := registry.New(pool, zap.NewNop())
	verif := stake.New(pool, lnMock, lnMock, stake.SlashConfig{}, zap.NewNop())
	srv := rpc.New(reg, nil, verif, nil, nil, nil, pool, zap.NewNop())
	return srv, func() {}
}

// TestRegisterNode_InsufficientStake verifies SRS-STAKE-01/LN-11: when the LN client
// reports no qualifying channel, RegisterNode returns "pending" with INSUFFICIENT_STAKE
// and the node is NOT activated.
func TestRegisterNode_InsufficientStake(t *testing.T) {
	lnMock := mock.New()
	srv, _ := buildServer(t, lnMock)

	ctx := context.Background()
	nf := testutil.NewNodeFixture(t)

	// Configure mock to return no channels → stake check fails.
	lnMock.SetChannels(nf.PubKeyHex, []lightning.Channel{})

	ts := time.Now().Unix()
	resp, err := srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:  nf.PubKeyHex,
		LnNodeUri:  nf.LNNodeURI,
		Timestamp:  ts,
		Signature:  nf.Sign(nf.LNNodeURI, "t1", ts),
		Capabilities: &coordinatorv1.NodeCapabilities{
			Tier:   "t1",
			VramGb: 8,
			RamGb:  16,
		},
	})
	if err != nil {
		t.Fatalf("RegisterNode: unexpected gRPC error: %v", err)
	}
	if resp.Status != "pending" {
		t.Errorf("status: got %q, want pending", resp.Status)
	}
	if resp.ErrorCode != "INSUFFICIENT_STAKE" {
		t.Errorf("error_code: got %q, want INSUFFICIENT_STAKE", resp.ErrorCode)
	}
}

// TestRegisterNode_SufficientStake verifies the happy path (SRS-STAKE-01/LN-11):
// a node with a qualifying channel is registered AND activated in one round-trip.
func TestRegisterNode_SufficientStake(t *testing.T) {
	lnMock := mock.New()
	srv, _ := buildServer(t, lnMock)

	ctx := context.Background()
	nf := testutil.NewNodeFixture(t)

	// Configure mock with a channel that meets the t1 minimum (100k sats).
	lnMock.SetChannels(nf.PubKeyHex, []lightning.Channel{{
		ChannelID:        "ch-ok",
		RemotePubkey:     nf.PubKeyHex,
		CapacitySats:     150_000,
		LocalBalanceSats: 150_000,
		Active:           true,
	}})

	ts := time.Now().Unix()
	resp, err := srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:  nf.PubKeyHex,
		LnNodeUri:  nf.LNNodeURI,
		Timestamp:  ts,
		Signature:  nf.Sign(nf.LNNodeURI, "t1", ts),
		Capabilities: &coordinatorv1.NodeCapabilities{
			Tier:   "t1",
			VramGb: 8,
			RamGb:  16,
		},
	})
	if err != nil {
		t.Fatalf("RegisterNode: %v", err)
	}
	if resp.Status != "active" {
		t.Errorf("status: got %q, want active", resp.Status)
	}
	if resp.ErrorCode != "" {
		t.Errorf("unexpected error_code: %q", resp.ErrorCode)
	}
	if resp.StakeSats != 150_000 {
		t.Errorf("stake_sats: got %d, want 150000", resp.StakeSats)
	}
}

// TestRegisterNode_SubMinimumStake verifies that a channel present but below tier
// minimum causes the node to remain pending.
func TestRegisterNode_SubMinimumStake(t *testing.T) {
	lnMock := mock.New()
	srv, _ := buildServer(t, lnMock)

	ctx := context.Background()
	nf := testutil.NewNodeFixture(t)

	// t2 needs 500k; give only 100k.
	lnMock.SetChannels(nf.PubKeyHex, []lightning.Channel{{
		ChannelID:        "ch-sub",
		RemotePubkey:     nf.PubKeyHex,
		CapacitySats:     100_000,
		LocalBalanceSats: 100_000,
		Active:           true,
	}})

	ts := time.Now().Unix()
	resp, err := srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:  nf.PubKeyHex,
		LnNodeUri:  nf.LNNodeURI,
		Timestamp:  ts,
		Signature:  nf.Sign(nf.LNNodeURI, "t2", ts),
		Capabilities: &coordinatorv1.NodeCapabilities{
			Tier:   "t2",
			VramGb: 24,
			RamGb:  64,
		},
	})
	if err != nil {
		t.Fatalf("RegisterNode: unexpected gRPC error: %v", err)
	}
	if resp.Status != "pending" {
		t.Errorf("status: got %q, want pending", resp.Status)
	}
	if resp.ErrorCode != "INSUFFICIENT_STAKE" {
		t.Errorf("error_code: got %q, want INSUFFICIENT_STAKE", resp.ErrorCode)
	}
}

// TestDeregisterNode_ValidSignature verifies SRS-STAKE-08: a node can voluntarily
// deregister when it provides a valid Ed25519 signature over the canonical message.
func TestDeregisterNode_ValidSignature(t *testing.T) {
	lnMock := mock.New()
	srv, _ := buildServer(t, lnMock)

	ctx := context.Background()
	nf := testutil.NewNodeFixture(t)

	lnMock.SetChannels(nf.PubKeyHex, []lightning.Channel{{
		ChannelID:        "ch-dereg",
		RemotePubkey:     nf.PubKeyHex,
		CapacitySats:     150_000,
		LocalBalanceSats: 150_000,
		Active:           true,
	}})
	ts := time.Now().Unix()
	regResp, err := srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:    nf.PubKeyHex,
		LnNodeUri:    nf.LNNodeURI,
		Timestamp:    ts,
		Signature:    nf.Sign(nf.LNNodeURI, "t1", ts),
		Capabilities: &coordinatorv1.NodeCapabilities{Tier: "t1", VramGb: 8, RamGb: 16},
	})
	if err != nil || regResp.Status != "active" {
		t.Fatalf("RegisterNode: err=%v status=%q", err, regResp.GetStatus())
	}
	nodeID := regResp.NodeId

	deregTs := time.Now().Unix()
	reason := "shutdown"
	msg := []byte(fmt.Sprintf("owm-deregister|%s|%s|%d", nodeID, reason, deregTs))
	sig := ed25519.Sign(nf.PrivKey, msg)

	resp, err := srv.DeregisterNode(ctx, &coordinatorv1.DeregisterNodeRequest{
		NodeId:    nodeID,
		Timestamp: deregTs,
		Reason:    reason,
		Signature: sig,
	})
	if err != nil {
		t.Fatalf("DeregisterNode: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected Success=true, got false: %s", resp.Message)
	}
}

// TestDeregisterNode_InvalidSignature verifies that a deregistration with a bad
// signature is rejected.
func TestDeregisterNode_InvalidSignature(t *testing.T) {
	lnMock := mock.New()
	srv, _ := buildServer(t, lnMock)

	ctx := context.Background()
	nf := testutil.NewNodeFixture(t)

	lnMock.SetChannels(nf.PubKeyHex, []lightning.Channel{{
		ChannelID:        "ch-bad-sig",
		RemotePubkey:     nf.PubKeyHex,
		CapacitySats:     150_000,
		LocalBalanceSats: 150_000,
		Active:           true,
	}})
	ts := time.Now().Unix()
	regResp, err := srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:    nf.PubKeyHex,
		LnNodeUri:    nf.LNNodeURI,
		Timestamp:    ts,
		Signature:    nf.Sign(nf.LNNodeURI, "t1", ts),
		Capabilities: &coordinatorv1.NodeCapabilities{Tier: "t1", VramGb: 8, RamGb: 16},
	})
	if err != nil || regResp.Status != "active" {
		t.Fatalf("RegisterNode: err=%v status=%q", err, regResp.GetStatus())
	}

	deregTs := time.Now().Unix()
	_, err = srv.DeregisterNode(ctx, &coordinatorv1.DeregisterNodeRequest{
		NodeId:    regResp.NodeId,
		Timestamp: deregTs,
		Reason:    "shutdown",
		Signature: []byte("not-a-real-signature"),
	})
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}

// TestRegisterNode_CooldownBlocked verifies SRS-STAKE-07: a slashed node whose
// cooldown has not yet expired is rejected with PERMISSION_DENIED.
func TestRegisterNode_CooldownBlocked(t *testing.T) {
	lnMock := mock.New()
	srv, _ := buildServer(t, lnMock)

	ctx := context.Background()
	nf := testutil.NewNodeFixture(t)

	// First: register and activate so the node exists in the DB.
	lnMock.SetChannels(nf.PubKeyHex, []lightning.Channel{{
		ChannelID:        "ch-cooldown",
		RemotePubkey:     nf.PubKeyHex,
		CapacitySats:     150_000,
		LocalBalanceSats: 150_000,
		Active:           true,
	}})
	ts := time.Now().Unix()
	regResp, err := srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:    nf.PubKeyHex,
		LnNodeUri:    nf.LNNodeURI,
		Timestamp:    ts,
		Signature:    nf.Sign(nf.LNNodeURI, "t1", ts),
		Capabilities: &coordinatorv1.NodeCapabilities{Tier: "t1", VramGb: 8, RamGb: 16},
	})
	if err != nil || regResp.Status != "active" {
		t.Fatalf("initial registration: err=%v status=%q", err, regResp.GetStatus())
	}

	// Insert a slashing event with a future cooldown expiry directly into DB.
	pool := testutil.MustDB(t)
	nodeID := regResp.NodeId
	if _, err := pool.Exec(ctx,
		`INSERT INTO slashing_events
		    (node_id, channel_id, tier, reason, evidence_hash, signal_count, cooldown_expires_at, coordinator_sig)
		 VALUES ($1::uuid, 'ch-cooldown', 't1', 'test', 'evidence', 3, now() + interval '30 days', 'pending')`,
		nodeID,
	); err != nil {
		t.Fatalf("insert slashing event: %v", err)
	}

	// Attempt re-registration — must be rejected.
	ts2 := time.Now().Unix()
	_, err = srv.RegisterNode(ctx, &coordinatorv1.RegisterNodeRequest{
		PublicKey:    nf.PubKeyHex,
		LnNodeUri:    nf.LNNodeURI,
		Timestamp:    ts2,
		Signature:    nf.Sign(nf.LNNodeURI, "t1", ts2),
		Capabilities: &coordinatorv1.NodeCapabilities{Tier: "t1", VramGb: 8, RamGb: 16},
	})
	if err == nil {
		t.Fatal("expected error for node in cooldown, got nil")
	}
	t.Logf("correctly rejected with: %v", err)
}
