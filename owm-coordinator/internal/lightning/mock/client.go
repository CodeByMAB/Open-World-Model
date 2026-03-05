// Package mock provides a no-op Lightning client for development and testing.
// It satisfies the lightning.Client interface without requiring a live LND/CLN node.
package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
)

// Client is a stub Lightning client that returns synthetic success responses.
// Use when OWM_DEV_MODE=true or in tests to avoid requiring a real LN node.
// SetChannels, FailPayments, and FailForceClose allow tests to control behavior.
type Client struct {
	mu            sync.RWMutex
	channels      map[string][]lightning.Channel // key: remotePubkeyHex
	FailPayments  bool                           // when true, SendPayment returns error
	FailForceClose bool                          // when true, ForceCloseChan returns error
}

// New returns a new mock Lightning client.
func New() *Client {
	return &Client{
		channels: make(map[string][]lightning.Channel),
	}
}

// SetChannels configures the channels returned by ListChannels for the given pubkey.
// Safe for concurrent use.
func (c *Client) SetChannels(pubkeyHex string, channels []lightning.Channel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.channels == nil {
		c.channels = make(map[string][]lightning.Channel)
	}
	c.channels[pubkeyHex] = channels
}

// ListChannels returns channels configured via SetChannels for remotePubkeyHex,
// or a single synthetic qualifying channel if none are set.
func (c *Client) ListChannels(ctx context.Context, remotePubkeyHex string) ([]lightning.Channel, error) {
	_ = ctx
	c.mu.RLock()
	chans, ok := c.channels[remotePubkeyHex]
	c.mu.RUnlock()
	if ok {
		return chans, nil
	}
	// Default: synthetic channel that meets minimum for any tier (t3 = 2M sats).
	return []lightning.Channel{{
		ChannelID:        "mock-dev-channel",
		RemotePubkey:     remotePubkeyHex,
		CapacitySats:     2_500_000,
		LocalBalanceSats: 2_500_000,
		Active:           true,
	}}, nil
}

// SendPayment simulates a successful payment unless FailPayments is true.
func (c *Client) SendPayment(ctx context.Context, req lightning.SendPaymentRequest) (*lightning.PaymentResult, error) {
	_ = ctx
	c.mu.RLock()
	fail := c.FailPayments
	c.mu.RUnlock()
	if fail {
		return nil, errors.New("mock: payments disabled for test")
	}
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

// ForceCloseChan is a no-op unless FailForceClose is true.
func (c *Client) ForceCloseChan(ctx context.Context, channelID string) error {
	_ = ctx
	_ = channelID
	c.mu.RLock()
	fail := c.FailForceClose
	c.mu.RUnlock()
	if fail {
		return errors.New("mock: force close disabled for test")
	}
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
