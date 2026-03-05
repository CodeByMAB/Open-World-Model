// Package lightning defines the LNClient interface and shared types used by
// the coordinator for payments, stake verification, and slashing.
// Concrete implementations (LND, CLN) live in sub-packages.
package lightning

import "context"

// Channel represents a Lightning payment channel relevant to OWM.
type Channel struct {
	ChannelID        string
	RemotePubkey     string
	CapacitySats     int64
	LocalBalanceSats int64
	Active           bool
}

// Client abstracts over Lightning Network implementations (LND, CLN).
// Different credential-scoped instances are injected for payment, readonly,
// and slashing operations — matching ADR-004's credential isolation design.
type Client interface {
	// ListChannels returns all channels with the given remote pubkey.
	// Used by the stake verifier with a readonly macaroon.
	ListChannels(ctx context.Context, remotePubkeyHex string) ([]Channel, error)

	// SendPayment dispatches a Lightning payment to a BOLT11 invoice or keysend.
	// Used by the payment dispatcher with a payment macaroon.
	SendPayment(ctx context.Context, req SendPaymentRequest) (*PaymentResult, error)

	// AddInvoice creates a BOLT11 invoice for incoming payments (commercial API).
	AddInvoice(ctx context.Context, amountSats int64, memo string, expirySeconds int64) (*Invoice, error)

	// ForceCloseChan initiates a unilateral channel close.
	// Used exclusively by the stake manager with the slashing macaroon.
	ForceCloseChan(ctx context.Context, channelID string) error

	// GetInfo returns basic node info (pubkey, alias, block height).
	GetInfo(ctx context.Context) (*NodeInfo, error)
}

// SendPaymentRequest carries parameters for a Lightning payment.
type SendPaymentRequest struct {
	DestPubkeyHex string // keysend destination
	AmountSats    int64
	Memo          string
	TimeoutSecs   int32
}

// PaymentResult is returned by SendPayment.
type PaymentResult struct {
	PaymentHash    string
	Preimage       string
	FeeSats        int64
	Status         string // "SUCCEEDED" | "FAILED" | "IN_FLIGHT"
	FailureReason  string
}

// Invoice represents a BOLT11 Lightning invoice.
type Invoice struct {
	PaymentRequest string // BOLT11 encoded
	PaymentHash    string
	ExpiresAt      int64 // Unix seconds
}

// NodeInfo contains basic Lightning node metadata.
type NodeInfo struct {
	PubkeyHex   string
	Alias       string
	BlockHeight uint32
}
