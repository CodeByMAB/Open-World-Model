// Package mock provides a no-op Lightning client for development and testing.
// It satisfies the lightning.Client interface without requiring a live LND/CLN node.
package mock

import (
	"context"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
)

// Client is a stub Lightning client that returns synthetic success responses.
// Use when OWM_DEV_MODE=true or in tests to avoid requiring a real LN node.
type Client struct{}

// New returns a new mock Lightning client.
func New() *Client {
	return &Client{}
}

// ListChannels returns a single synthetic channel meeting tier minimums.
// RemotePubkeyHex is ignored; the mock always returns a qualifying channel.
func (c *Client) ListChannels(ctx context.Context, remotePubkeyHex string) ([]lightning.Channel, error) {
	_ = ctx
	_ = remotePubkeyHex
	// Synthetic channel that meets minimum for any tier (t3 = 2M sats).
	return []lightning.Channel{{
		ChannelID:        "mock-dev-channel",
		RemotePubkey:     remotePubkeyHex,
		CapacitySats:     2_500_000,
		LocalBalanceSats: 2_500_000,
		Active:           true,
	}}, nil
}

// SendPayment simulates a successful payment (no-op).
func (c *Client) SendPayment(ctx context.Context, req lightning.SendPaymentRequest) (*lightning.PaymentResult, error) {
	_ = ctx
	_ = req
	return &lightning.PaymentResult{
		PaymentHash:   "mock-payment-hash",
		Preimage:      "mock-preimage",
		FeeSats:       0,
		Status:        "SUCCEEDED",
		FailureReason: "",
	}, nil
}

// AddInvoice returns a synthetic BOLT11 invoice string.
func (c *Client) AddInvoice(ctx context.Context, amountSats int64, memo string, expirySeconds int64) (*lightning.Invoice, error) {
	_ = ctx
	return &lightning.Invoice{
		PaymentRequest: "mock-bolt11-invoice",
		PaymentHash:    "mock-invoice-hash",
		ExpiresAt:     0,
	}, nil
}

// ForceCloseChan is a no-op; no real channel is closed.
func (c *Client) ForceCloseChan(ctx context.Context, channelID string) error {
	_ = ctx
	_ = channelID
	return nil
}

// GetInfo returns synthetic node info.
func (c *Client) GetInfo(ctx context.Context) (*lightning.NodeInfo, error) {
	_ = ctx
	return &lightning.NodeInfo{
		PubkeyHex:   "mock-pubkey-hex",
		Alias:       "owm-mock-dev",
		BlockHeight: 0,
	}, nil
}

// Ensure Client implements lightning.Client at compile time.
var _ lightning.Client = (*Client)(nil)
