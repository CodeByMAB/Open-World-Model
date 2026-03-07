package observer

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestClient_Submit_NoBlockOnPayment(t *testing.T) {
	// Failure-isolation: when Observer submission fails, we only log; payment
	// is already marked paid. This test verifies Submit returns error and does
	// not panic when the API returns 500.
	_, priv, _ := ed25519.GenerateKey(nil)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer svr.Close()

	client, err := NewClient(ClientConfig{
		APIEndpoint:      svr.URL,
		SigningKeyPEMOrHex: priv,
		Log:              zap.NewNop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client.SetSenderPubkeyHash("sender_hash_hex")
	receipt := Receipt{
		PaymentRail:           PaymentRailLightning,
		SettlementReference:   "preimage",
		ReceiverPublicKeyHash: "receiver_hash",
		AmountBucket:          "1-100",
	}
	_, err = client.Submit(context.Background(), receipt)
	if err == nil {
		t.Error("expected error when API returns 500")
	}
}

func TestClient_Submit_SuccessReturnsReceiptID(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"receipt_id":"obs_abc123"}`))
	}))
	defer svr.Close()

	client, err := NewClient(ClientConfig{
		APIEndpoint:      svr.URL,
		SigningKeyPEMOrHex: priv,
		Log:              zap.NewNop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client.SetSenderPubkeyHash("sender_sha256_hex")
	receipt := Receipt{
		PaymentRail:           PaymentRailLightning,
		SettlementReference:   "preimage_xyz",
		ReceiverPublicKeyHash: "receiver_sha256",
		AmountBucket:          "101-1000",
	}
	id, err := client.Submit(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if id != "obs_abc123" {
		t.Errorf("got receipt_id %q, want obs_abc123", id)
	}
}

func TestClient_Submit_2xxStrictParsing(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	makeClient := func(svr *httptest.Server) *Client {
		c, err := NewClient(ClientConfig{
			APIEndpoint:        strings.TrimRight(svr.URL, "/"),
			SigningKeyPEMOrHex: priv,
			Log:                zap.NewNop(),
		})
		if err != nil {
			t.Fatal(err)
		}
		c.SetSenderPubkeyHash("sender_hash")
		return c
	}

	t.Run("malformed JSON returns error", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not json`))
		}))
		defer svr.Close()
		client := makeClient(svr)
		_, err := client.Submit(context.Background(), Receipt{
			PaymentRail:           PaymentRailLightning,
			SettlementReference:   "x",
			ReceiverPublicKeyHash: "r",
			AmountBucket:          "1-100",
		})
		if err == nil {
			t.Error("expected error when 2xx body is not valid JSON")
		}
	})

	t.Run("no receipt_id or id returns error", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		defer svr.Close()
		client := makeClient(svr)
		_, err := client.Submit(context.Background(), Receipt{
			PaymentRail:           PaymentRailLightning,
			SettlementReference:   "x",
			ReceiverPublicKeyHash: "r",
			AmountBucket:          "1-100",
		})
		if err == nil {
			t.Error("expected error when 2xx has no receipt_id or id")
		}
	})
}

func TestDecodeSigningKey_128CharPrivateKeyHex(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	hexKey := hex.EncodeToString(priv) // 128 hex chars for 64-byte key
	decoded, err := decodeSigningKey([]byte(hexKey))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != ed25519.PrivateKeySize {
		t.Errorf("decoded key length %d, want %d", len(decoded), ed25519.PrivateKeySize)
	}
	if string(decoded) != string(priv) {
		t.Error("decoded key should match original 64-byte key")
	}
}

func TestDecodeSigningKey_64CharSeedHex(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	hexKey := hex.EncodeToString(seed) // 64 hex chars = 32-byte seed
	decoded, err := decodeSigningKey([]byte(hexKey))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != ed25519.PrivateKeySize {
		t.Errorf("decoded key length %d, want %d (seed should be expanded)", len(decoded), ed25519.PrivateKeySize)
	}
	// Expanded key from seed should be deterministic.
	expected := ed25519.NewKeyFromSeed(seed)
	if string(decoded) != string(expected) {
		t.Error("64-char seed hex should expand to same key as ed25519.NewKeyFromSeed(seed)")
	}
}

func TestDecodeSigningKey_InvalidLengthHex(t *testing.T) {
	// Odd length hex
	_, err := decodeSigningKey([]byte("abc"))
	if err == nil {
		t.Error("expected error for invalid hex length")
	}
	// 48 hex chars = 24 bytes, not 32 or 64
	_, err = decodeSigningKey([]byte(hex.EncodeToString(make([]byte, 24))))
	if err == nil {
		t.Error("expected error for 24-byte hex (invalid length)")
	}
	// 96 hex chars = 48 bytes
	_, err = decodeSigningKey([]byte(hex.EncodeToString(make([]byte, 48))))
	if err == nil {
		t.Error("expected error for 48-byte hex (invalid length)")
	}
}

func TestNewClient_RejectsInvalidKeyFormat(t *testing.T) {
	_, err := NewClient(ClientConfig{
		APIEndpoint:        "https://observer.example.com",
		SigningKeyPEMOrHex: []byte(hex.EncodeToString(make([]byte, 24))), // 48 hex chars = invalid length
		Log:                zap.NewNop(),
	})
	if err == nil {
		t.Error("NewClient should fail for invalid key format (24-byte hex)")
	}
}
