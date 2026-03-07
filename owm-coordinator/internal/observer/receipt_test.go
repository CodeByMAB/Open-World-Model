package observer

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestAmountBucket(t *testing.T) {
	tests := []struct {
		sats  int64
		want  string
	}{
		{0, "0"},
		{1, "1-100"},
		{100, "1-100"},
		{101, "101-1000"},
		{1000, "101-1000"},
		{1001, "1001-10000"},
		{10000, "1001-10000"},
		{10001, "10001+"},
		{999999, "10001+"},
	}
	for _, tt := range tests {
		got := AmountBucket(tt.sats)
		if got != tt.want {
			t.Errorf("AmountBucket(%d) = %q, want %q", tt.sats, got, tt.want)
		}
	}
}

func TestComputeReceiptHash_Determinism(t *testing.T) {
	r := Receipt{
		PaymentRail:           PaymentRailLightning,
		SettlementReference:   "preimage123",
		SenderPublicKeyHash:   "sender_hash_hex",
		ReceiverPublicKeyHash: "receiver_hash_hex",
		AmountBucket:          "101-1000",
	}
	h1 := ComputeReceiptHash(r)
	h2 := ComputeReceiptHash(r)
	if h1 != h2 {
		t.Errorf("receipt hash not deterministic: %q vs %q", h1, h2)
	}
	// Change one field and hash must change
	r.AmountBucket = "1-100"
	h3 := ComputeReceiptHash(r)
	if h1 == h3 {
		t.Error("receipt hash should change when payload changes")
	}
}

func TestSignReceipt_AndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	r := Receipt{
		PaymentRail:           PaymentRailLightning,
		SettlementReference:   "preimage_abc",
		SenderPublicKeyHash:   "aa",
		ReceiverPublicKeyHash: "bb",
		AmountBucket:          "1001-10000",
	}
	if err := SignReceipt(&r, priv); err != nil {
		t.Fatal(err)
	}
	if r.ReceiptHash == "" || r.Signature == "" {
		t.Error("SignReceipt should set ReceiptHash and Signature")
	}
	if !VerifySignature(r, pub) {
		t.Error("VerifySignature failed for valid signed receipt")
	}
	// Tampered payload: recompute hash for tampered data; original signature must not verify.
	r.SettlementReference = "tampered"
	r.ReceiptHash = ComputeReceiptHash(r)
	if VerifySignature(r, pub) {
		t.Error("VerifySignature should fail when payload was tampered (signature was over original hash)")
	}
}

func TestPubkeyHashHex(t *testing.T) {
	// 32-byte pubkey = 64 hex chars
	hexStr := hex.EncodeToString(make([]byte, 32))
	got := PubkeyHashHex(hexStr)
	if len(got) != 64 {
		t.Errorf("PubkeyHashHex: expected 64-char hex, got len %d", len(got))
	}
	invalid := PubkeyHashHex("not-hex")
	if invalid != "" {
		t.Errorf("PubkeyHashHex(invalid) should return empty, got %q", invalid)
	}
}
