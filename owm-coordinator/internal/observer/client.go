package observer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	defaultHTTPTimeout = 15 * time.Second
)

// Client submits signed payment receipts to the Observer Registry.
// It is constructed with the treasury node pubkey hash (from GetInfo at startup)
// and the coordinator's Ed25519 signing key.
type Client struct {
	apiEndpoint        string
	senderPubkeyHash   string
	signingKey         []byte
	httpClient         *http.Client
	log                *zap.Logger
}

// ClientConfig configures the Observer client.
type ClientConfig struct {
	APIEndpoint      string
	SigningKeyPEMOrHex []byte
	Log              *zap.Logger
}

// NewClient builds an Observer client. TreasuryPubkeyHash must be the
// SHA-256 hex hash of the Lightning node's pubkey (sender of payments).
// SigningKeyPEMOrHex is the coordinator's Ed25519 private key (32-byte seed
// or 64-byte full key, raw or hex, or PEM-encoded). Invalid key length fails
// at startup.
func NewClient(cfg ClientConfig) (*Client, error) {
	key, err := decodeSigningKey(cfg.SigningKeyPEMOrHex)
	if err != nil {
		return nil, fmt.Errorf("observer: signing key: %w", err)
	}
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("observer: signing key: invalid length %d (need %d)", len(key), ed25519.PrivateKeySize)
	}
	if cfg.Log == nil {
		cfg.Log = zap.NewNop()
	}
	return &Client{
		apiEndpoint:      strings.TrimRight(cfg.APIEndpoint, "/"),
		signingKey:       key,
		httpClient:       &http.Client{Timeout: defaultHTTPTimeout},
		log:              cfg.Log,
	}, nil
}

// SetSenderPubkeyHash sets the treasury (sender) pubkey hash. Called by
// main after GetInfo(lnReadonly) to avoid observer importing registry.
func (c *Client) SetSenderPubkeyHash(hashHex string) {
	c.senderPubkeyHash = strings.TrimSpace(hashHex)
}

// Submit sends the signed receipt to the Observer Registry. It returns the
// receipt ID returned by the API, or an error. Failures are not retried;
// callers should log and optionally persist for manual resubmission.
func (c *Client) Submit(ctx context.Context, r Receipt) (receiptID string, err error) {
	r.SenderPublicKeyHash = c.senderPubkeyHash
	if r.SenderPublicKeyHash == "" {
		return "", fmt.Errorf("observer: sender_public_key_hash not set (call SetSenderPubkeyHash first)")
	}
	if err := SignReceipt(&r, ed25519.PrivateKey(c.signingKey)); err != nil {
		return "", err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("observer: marshal receipt: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiEndpoint+"/v1/receipts", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("observer: POST receipts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		c.log.Warn("observer submission failed",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)),
		)
		return "", fmt.Errorf("observer: API returned %d: %s", resp.StatusCode, string(respBody))
	}
	var out struct {
		ReceiptID string `json:"receipt_id"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		excerpt := string(respBody)
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "..."
		}
		return "", fmt.Errorf("observer: API %d response JSON invalid: %w (body: %s)", resp.StatusCode, err, excerpt)
	}
	if out.ReceiptID != "" {
		return out.ReceiptID, nil
	}
	if out.ID != "" {
		return out.ID, nil
	}
	excerpt := string(respBody)
	if len(excerpt) > 200 {
		excerpt = excerpt[:200] + "..."
	}
	return "", fmt.Errorf("observer: API %d success but no receipt_id or id in response (body: %s)", resp.StatusCode, excerpt)
}

// decodeSigningKey returns a 64-byte Ed25519 private key from PEM, hex, or raw.
// 32-byte seed material (64-char hex or 32 raw bytes) is expanded via Ed25519
// seed expansion so SignReceipt receives a full private key.
func decodeSigningKey(data []byte) ([]byte, error) {
	// Raw binary: check BEFORE TrimSpace so whitespace bytes (0x0A, 0x20, …)
	// at the key boundary are not stripped. Hex strings (all bytes in [0-9a-fA-F])
	// of the same length are intentionally excluded and handled below.
	if (len(data) == 32 || len(data) == ed25519.PrivateKeySize) && !hexEncoded(data) {
		return normalizeToFullPrivateKey(data)
	}
	data = bytes.TrimSpace(data)
	// Hex: 64 chars = 32-byte seed, 128 chars = 64-byte full key.
	if len(data) >= 64 && len(data)%2 == 0 && hexEncoded(data) {
		decoded, err := hex.DecodeString(string(data))
		if err != nil {
			return nil, err
		}
		return normalizeToFullPrivateKey(decoded)
	}
	// PEM: look for PRIVATE KEY and decode base64 block (minimal PEM parse).
	if bytes.Contains(data, []byte("PRIVATE KEY")) {
		return decodePEMEd25519PrivateKey(data)
	}
	return nil, fmt.Errorf("invalid key format: need 32- or 64-byte raw, 64- or 128-char hex, or PEM")
}

// normalizeToFullPrivateKey expands a 32-byte Ed25519 seed to a 64-byte private
// key, or returns the key unchanged if it is already 64 bytes.
func normalizeToFullPrivateKey(key []byte) ([]byte, error) {
	switch len(key) {
	case ed25519.PrivateKeySize:
		return key, nil
	case 32:
		return ed25519.NewKeyFromSeed(key), nil
	default:
		return nil, fmt.Errorf("invalid key length %d (need 32 or %d bytes)", len(key), ed25519.PrivateKeySize)
	}
}

func hexEncoded(b []byte) bool {
	for _, ch := range b {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}

// decodePEMEd25519PrivateKey extracts Ed25519 private key from a PEM block.
func decodePEMEd25519PrivateKey(data []byte) ([]byte, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8: %w", err)
	}
	k, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not Ed25519")
	}
	return k, nil
}
