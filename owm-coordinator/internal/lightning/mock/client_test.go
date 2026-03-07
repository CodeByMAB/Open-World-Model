package mock

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/observer"
	"go.uber.org/zap"
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

// TestMockClient_GetInfo_DevModeObserverPath verifies that mock GetInfo returns
// a valid hex pubkey so observer.PubkeyHashHex is non-empty and Client.Submit
// proceeds past sender-hash validation (dev-mode observer path).
func TestMockClient_GetInfo_DevModeObserverPath(t *testing.T) {
	ctx := context.Background()
	mockLN := New()
	info, err := mockLN.GetInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hash := observer.PubkeyHashHex(info.PubkeyHex)
	if hash == "" {
		t.Fatal("PubkeyHashHex(mock GetInfo.PubkeyHex) must be non-empty for dev-mode observer")
	}
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"receipt_id":"dev-obs-123"}`))
	}))
	defer svr.Close()
	_, priv, _ := ed25519.GenerateKey(nil)
	obsClient, err := observer.NewClient(observer.ClientConfig{
		APIEndpoint:        svr.URL,
		SigningKeyPEMOrHex: priv,
		Log:                zap.NewNop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	obsClient.SetSenderPubkeyHash(hash)
	receipt := observer.Receipt{
		PaymentRail:           observer.PaymentRailLightning,
		SettlementReference:   "mock-preimage",
		ReceiverPublicKeyHash: observer.PubkeyHashHex("02abcdef"),
		AmountBucket:          "1-100",
	}
	receiptID, err := obsClient.Submit(ctx, receipt)
	if err != nil {
		t.Fatalf("Submit must proceed past sender-hash validation in dev mode: %v", err)
	}
	if receiptID != "dev-obs-123" {
		t.Errorf("got receipt_id %q, want dev-obs-123", receiptID)
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
