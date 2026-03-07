// Package observer implements the Observer Protocol v0.1 cryptographic payment
// receipt submission. Receipts are signed by the coordinator and submitted
// to the Observer Registry for verification and audit.
package observer

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	PaymentRailLightning = "lightning"
)

// AmountBucket maps reward_sats to Observer Protocol amount buckets.
// Buckets are inclusive on the lower bound.
var amountBucketRanges = []struct {
	label string
	max   int64
}{
	{"1-100", 100},
	{"101-1000", 1000},
	{"1001-10000", 10000},
	{"10001+", -1}, // no upper bound
}

// AmountBucket returns the Observer Protocol amount_bucket string for sats.
func AmountBucket(sats int64) string {
	if sats < 1 {
		return "0"
	}
	for _, b := range amountBucketRanges {
		if b.max < 0 || sats <= b.max {
			return b.label
		}
	}
	return "10001+"
}

// Receipt is the Observer Protocol v0.1 payment receipt payload.
// ReceiptHash is computed over the canonical JSON of the fields that precede
// the signature; Signature is Ed25519(coordinator_private_key, receipt_hash).
type Receipt struct {
	PaymentRail             string `json:"payment_rail"`
	SettlementReference     string `json:"settlement_reference"`
	SenderPublicKeyHash     string `json:"sender_public_key_hash"`
	ReceiverPublicKeyHash   string `json:"receiver_public_key_hash"`
	AmountBucket            string `json:"amount_bucket"`
	ReceiptHash             string `json:"receipt_hash"`
	Signature               string `json:"signature"`
}

// receiptFieldsForHash is the struct used to compute receipt_hash (no signature).
type receiptFieldsForHash struct {
	PaymentRail           string `json:"payment_rail"`
	SettlementReference   string `json:"settlement_reference"`
	SenderPublicKeyHash   string `json:"sender_public_key_hash"`
	ReceiverPublicKeyHash string `json:"receiver_public_key_hash"`
	AmountBucket          string `json:"amount_bucket"`
}

// ComputeReceiptHash returns the SHA-256 hex hash of the canonical receipt
// payload (all fields except receipt_hash and signature). Same inputs must
// always produce the same hash for signature verification.
func ComputeReceiptHash(r Receipt) string {
	payload := receiptFieldsForHash{
		PaymentRail:           r.PaymentRail,
		SettlementReference:   r.SettlementReference,
		SenderPublicKeyHash:   r.SenderPublicKeyHash,
		ReceiverPublicKeyHash: r.ReceiverPublicKeyHash,
		AmountBucket:          r.AmountBucket,
	}
	canonical := canonicalJSON(payload)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// canonicalJSON marshals a struct to JSON with sorted keys for determinism.
func canonicalJSON(v interface{}) []byte {
	m, err := structToMap(v)
	if err != nil {
		panic(err)
	}
	return marshalMapSorted(m)
}

func structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func marshalMapSorted(m map[string]interface{}) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(jsonString(k))
		b.WriteByte(':')
		val := m[k]
		switch v := val.(type) {
		case string:
			b.WriteString(jsonString(v))
		default:
			inner, _ := json.Marshal(val)
			b.Write(inner)
		}
	}
	b.WriteByte('}')
	return []byte(b.String())
}

func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// SignReceipt sets ReceiptHash and Signature on r using the given Ed25519
// private key. The key must be 32 bytes (ed25519.PrivateKey size).
func SignReceipt(r *Receipt, privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("observer: invalid ed25519 key size %d", len(privateKey))
	}
	r.ReceiptHash = ComputeReceiptHash(*r)
	hashBytes, err := hex.DecodeString(r.ReceiptHash)
	if err != nil {
		return fmt.Errorf("observer: decode receipt_hash: %w", err)
	}
	sig := ed25519.Sign(privateKey, hashBytes)
	r.Signature = hex.EncodeToString(sig)
	return nil
}

// PubkeyHashHex returns the SHA-256 hash of the pubkey bytes (hex-decoded),
// encoded as hex. Used for sender_public_key_hash and receiver_public_key_hash.
func PubkeyHashHex(pubkeyHex string) string {
	raw, err := hex.DecodeString(strings.TrimSpace(pubkeyHex))
	if err != nil || len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// VerifySignature verifies that r.Signature is a valid Ed25519 signature over
// r.ReceiptHash using the given public key. Used in tests.
func VerifySignature(r Receipt, publicKey ed25519.PublicKey) bool {
	hashBytes, err := hex.DecodeString(r.ReceiptHash)
	if err != nil || len(hashBytes) != sha256.Size {
		return false
	}
	sigBytes, err := hex.DecodeString(r.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(publicKey, hashBytes, sigBytes)
}
