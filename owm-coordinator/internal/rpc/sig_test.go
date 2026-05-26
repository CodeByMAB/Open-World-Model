package rpc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// ─── canonicalTaskResultMessage ──────────────────────────────────────────────

func TestCanonicalTaskResultMessage_Format(t *testing.T) {
	taskID := "550e8400-e29b-41d4-a716-446655440000"
	hashHex := strings.Repeat("ab", 32) // 64-char hex
	got := string(canonicalTaskResultMessage(taskID, hashHex))
	want := "owm-task-result|" + taskID + "|" + hashHex
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonicalTaskResultMessage_Deterministic(t *testing.T) {
	a := canonicalTaskResultMessage("task-1", "deadbeef")
	b := canonicalTaskResultMessage("task-1", "deadbeef")
	if string(a) != string(b) {
		t.Error("not deterministic")
	}
}

func TestCanonicalTaskResultMessage_TaskIDDistinct(t *testing.T) {
	a := string(canonicalTaskResultMessage("task-1", "hash"))
	b := string(canonicalTaskResultMessage("task-2", "hash"))
	if a == b {
		t.Error("different task IDs must produce different messages")
	}
}

func TestCanonicalTaskResultMessage_HashDistinct(t *testing.T) {
	a := string(canonicalTaskResultMessage("task-1", "hash-a"))
	b := string(canonicalTaskResultMessage("task-1", "hash-b"))
	if a == b {
		t.Error("different output hashes must produce different messages")
	}
}

// ─── verifyTaskResultSig ─────────────────────────────────────────────────────

func mustGenKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

func signTaskResult(t *testing.T, priv ed25519.PrivateKey, taskID string, outputHash []byte) []byte {
	t.Helper()
	msg := canonicalTaskResultMessage(taskID, hex.EncodeToString(outputHash))
	return ed25519.Sign(priv, msg)
}

func TestVerifyTaskResultSig_Valid(t *testing.T) {
	pub, priv := mustGenKey(t)
	taskID := "550e8400-e29b-41d4-a716-446655440000"
	outputHash := make([]byte, 32)
	sig := signTaskResult(t, priv, taskID, outputHash)

	if err := verifyTaskResultSig(hex.EncodeToString(pub), taskID, outputHash, sig); err != nil {
		t.Fatalf("expected valid signature to pass: %v", err)
	}
}

func TestVerifyTaskResultSig_EmptySig(t *testing.T) {
	pub, _ := mustGenKey(t)
	err := verifyTaskResultSig(hex.EncodeToString(pub), "task-1", make([]byte, 32), nil)
	if err == nil {
		t.Fatal("expected error for nil signature")
	}
	err = verifyTaskResultSig(hex.EncodeToString(pub), "task-1", make([]byte, 32), []byte{})
	if err == nil {
		t.Fatal("expected error for empty signature")
	}
}

func TestVerifyTaskResultSig_WrongKey(t *testing.T) {
	_, priv := mustGenKey(t)
	wrongPub, _ := mustGenKey(t)
	taskID := "task-abc"
	outputHash := make([]byte, 32)
	sig := signTaskResult(t, priv, taskID, outputHash)

	err := verifyTaskResultSig(hex.EncodeToString(wrongPub), taskID, outputHash, sig)
	if err == nil {
		t.Fatal("expected error: signature was made with a different key")
	}
}

func TestVerifyTaskResultSig_TamperedOutputHash(t *testing.T) {
	pub, priv := mustGenKey(t)
	taskID := "task-abc"
	outputHash := make([]byte, 32)
	sig := signTaskResult(t, priv, taskID, outputHash)

	// Flip one byte in the output hash after signing.
	tampered := make([]byte, 32)
	copy(tampered, outputHash)
	tampered[0] ^= 0xFF

	err := verifyTaskResultSig(hex.EncodeToString(pub), taskID, tampered, sig)
	if err == nil {
		t.Fatal("expected error: output hash was tampered after signing")
	}
}

func TestVerifyTaskResultSig_TamperedTaskID(t *testing.T) {
	pub, priv := mustGenKey(t)
	outputHash := make([]byte, 32)
	sig := signTaskResult(t, priv, "original-task-id", outputHash)

	err := verifyTaskResultSig(hex.EncodeToString(pub), "different-task-id", outputHash, sig)
	if err == nil {
		t.Fatal("expected error: task_id was changed after signing")
	}
}

func TestVerifyTaskResultSig_TamperedSig(t *testing.T) {
	pub, priv := mustGenKey(t)
	taskID := "task-abc"
	outputHash := make([]byte, 32)
	sig := signTaskResult(t, priv, taskID, outputHash)

	// Flip the first byte of the signature.
	corrupt := make([]byte, len(sig))
	copy(corrupt, sig)
	corrupt[0] ^= 0xFF

	err := verifyTaskResultSig(hex.EncodeToString(pub), taskID, outputHash, corrupt)
	if err == nil {
		t.Fatal("expected error: signature was corrupted")
	}
}

func TestVerifyTaskResultSig_InvalidPubKeyHex(t *testing.T) {
	err := verifyTaskResultSig("not-hex!!", "task", make([]byte, 32), make([]byte, 64))
	if err == nil {
		t.Fatal("expected error for non-hex public key")
	}
}

func TestVerifyTaskResultSig_WrongPubKeyLength(t *testing.T) {
	// Valid hex but only 16 bytes — not an Ed25519 public key.
	shortKey := hex.EncodeToString(make([]byte, 16))
	err := verifyTaskResultSig(shortKey, "task", make([]byte, 32), make([]byte, 64))
	if err == nil {
		t.Fatal("expected error for public key with wrong length")
	}
}

// ─── canonicalDeregisterMessage ──────────────────────────────────────────────

func TestCanonicalDeregisterMessage_Format(t *testing.T) {
	nodeID := "550e8400-e29b-41d4-a716-446655440000"
	reason := "shutdown"
	ts := int64(1700000000)
	got := string(canonicalDeregisterMessage(nodeID, reason, ts))
	want := "owm-deregister|550e8400-e29b-41d4-a716-446655440000|shutdown|1700000000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonicalDeregisterMessage_NodeIDDistinct(t *testing.T) {
	a := string(canonicalDeregisterMessage("node-1", "reason", 100))
	b := string(canonicalDeregisterMessage("node-2", "reason", 100))
	if a == b {
		t.Error("different node IDs must produce different messages")
	}
}

func TestCanonicalDeregisterMessage_TimestampDistinct(t *testing.T) {
	a := string(canonicalDeregisterMessage("node-1", "reason", 100))
	b := string(canonicalDeregisterMessage("node-1", "reason", 101))
	if a == b {
		t.Error("different timestamps must produce different messages")
	}
}

// ─── extractClientIP ─────────────────────────────────────────────────────────

func TestExtractClientIP_NoContext(t *testing.T) {
	ctx := context.Background()
	ip := extractClientIP(ctx)
	if ip != "" {
		t.Errorf("expected empty IP from bare context, got %q", ip)
	}
}

// ─── checkRegistrationRateLimit ──────────────────────────────────────────────

func TestCheckRegistrationRateLimit_NilRedis(t *testing.T) {
	// When rdb is nil, rate limiting is disabled; must return nil (no error).
	s := &Server{rdb: nil}
	err := s.checkRegistrationRateLimit(context.Background(), "deadbeef")
	if err != nil {
		t.Errorf("expected nil with no Redis, got %v", err)
	}
}
