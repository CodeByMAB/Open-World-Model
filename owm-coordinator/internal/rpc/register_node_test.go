package rpc_test

import (
	"context"
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
