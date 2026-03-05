package mock

import (
	"context"
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
)

func TestClient_ListChannels(t *testing.T) {
	ctx := context.Background()
	c := New()
	chans, err := c.ListChannels(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(chans) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chans))
	}
	if chans[0].ChannelID != "mock-dev-channel" || !chans[0].Active {
		t.Errorf("unexpected channel: %+v", chans[0])
	}
	if chans[0].LocalBalanceSats < 2_000_000 {
		t.Errorf("mock channel should meet t3 minimum (2M), got %d", chans[0].LocalBalanceSats)
	}
}

func TestClient_SendPayment(t *testing.T) {
	ctx := context.Background()
	c := New()
	res, err := c.SendPayment(ctx, lightning.SendPaymentRequest{
		DestPubkeyHex: "pubkey",
		AmountSats:    100,
		Memo:          "memo",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("SendPayment: %v", err)
	}
	if res.Status != "SUCCEEDED" {
		t.Errorf("expected SUCCEEDED, got %s", res.Status)
	}
}

func TestClient_ForceCloseChan(t *testing.T) {
	ctx := context.Background()
	c := New()
	if err := c.ForceCloseChan(ctx, "any-channel-id"); err != nil {
		t.Fatalf("ForceCloseChan: %v", err)
	}
}
