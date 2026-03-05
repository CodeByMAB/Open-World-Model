package mock

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
)

func TestMockClient_SetChannels_ListChannels(t *testing.T) {
	ctx := context.Background()
	c := New()
	pubkey := "abc123"
	chans := []lightning.Channel{{
		ChannelID:        "ch1",
		RemotePubkey:     pubkey,
		CapacitySats:     500_000,
		LocalBalanceSats: 400_000,
		Active:           true,
	}}
	c.SetChannels(pubkey, chans)
	got, err := c.ListChannels(ctx, pubkey)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ChannelID != "ch1" || got[0].CapacitySats != 500_000 {
		t.Errorf("ListChannels: got %+v", got)
	}
	// Unset pubkey returns default synthetic channel
	got2, _ := c.ListChannels(ctx, "other")
	if len(got2) != 1 || got2[0].ChannelID != "mock-dev-channel" {
		t.Errorf("default channel: got %+v", got2)
	}
}

func TestMockClient_FailPayments(t *testing.T) {
	ctx := context.Background()
	c := New()
	c.FailPayments = true
	_, err := c.SendPayment(ctx, lightning.SendPaymentRequest{AmountSats: 100})
	if err == nil {
		t.Error("expected error when FailPayments=true")
	}
	c.FailPayments = false
	res, err := c.SendPayment(ctx, lightning.SendPaymentRequest{})
	if err != nil || res.Status != "SUCCEEDED" {
		t.Errorf("expected success: err=%v res=%+v", err, res)
	}
}

func TestMockClient_FailForceClose(t *testing.T) {
	ctx := context.Background()
	c := New()
	c.FailForceClose = true
	err := c.ForceCloseChan(ctx, "ch1")
	if err == nil {
		t.Error("expected error when FailForceClose=true")
	}
	c.FailForceClose = false
	err = c.ForceCloseChan(ctx, "ch1")
	if err != nil {
		t.Errorf("expected nil: %v", err)
	}
}

func TestMockClient_Concurrency(t *testing.T) {
	ctx := context.Background()
	c := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		pubkey := fmt.Sprintf("pubkey-%d", i)
		chans := []lightning.Channel{{ChannelID: pubkey, RemotePubkey: pubkey, CapacitySats: 100_000, LocalBalanceSats: 100_000, Active: true}}
		go func() {
			defer wg.Done()
			c.SetChannels(pubkey, chans)
		}()
		go func() {
			defer wg.Done()
			_, _ = c.ListChannels(ctx, pubkey)
		}()
	}
	wg.Wait()
}
