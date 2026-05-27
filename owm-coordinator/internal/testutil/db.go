// Package testutil provides helpers for integration tests that require a live
// PostgreSQL database. Set OWM_TEST_DSN to a Postgres DSN to run them; tests
// are skipped automatically when the variable is absent.
package testutil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// MustDB returns a live pgxpool.Pool for integration tests.
// Skips the test if OWM_TEST_DSN is unset; runs all pending migrations.
// The pool is closed automatically at test cleanup.
func MustDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("OWM_TEST_DSN")
	if dsn == "" {
		t.Skip("integration test: set OWM_TEST_DSN to run")
	}

	// Navigate from this source file to the migrations directory.
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	migrationsDir, _ = filepath.Abs(migrationsDir)

	// golang-migrate uses the pq driver which requires sslmode=disable on
	// servers without TLS (e.g. the CI Postgres service container).
	migrDSN := dsn
	if !strings.Contains(migrDSN, "sslmode=") {
		if strings.Contains(migrDSN, "?") {
			migrDSN += "&sslmode=disable"
		} else {
			migrDSN += "?sslmode=disable"
		}
	}
	m, err := migrate.New("file://"+migrationsDir, migrDSN)
	if err != nil {
		t.Fatalf("testutil.MustDB: migrate.New: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("testutil.MustDB: migrate.Up: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("testutil.MustDB: pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TruncateAll removes all rows from every OWM application table and resets
// sequences. Call at the start of each integration test for isolation.
func TruncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE
			misbehavior_signals,
			slashing_events,
			node_stakes,
			fl_participants,
			model_versions,
			bounties,
			tasks,
			fl_rounds,
			nodes
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("TruncateAll: %v", err)
	}
}

// Logger returns a no-op zap logger suitable for unit and integration tests.
func Logger() *zap.Logger {
	return zap.NewNop()
}

// NodeFixture holds an Ed25519 key pair and a synthetic LN node URI for tests.
type NodeFixture struct {
	PubKeyHex string
	PubKey    ed25519.PublicKey
	PrivKey   ed25519.PrivateKey
	LNNodeURI string
}

// NewNodeFixture generates a fresh Ed25519 key pair.
func NewNodeFixture(t *testing.T) *NodeFixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("NewNodeFixture: %v", err)
	}
	pubHex := hex.EncodeToString(pub)
	return &NodeFixture{
		PubKeyHex: pubHex,
		PubKey:    pub,
		PrivKey:   priv,
		LNNodeURI: pubHex[:20] + "@127.0.0.1:9735",
	}
}

// Sign returns an Ed25519 signature over the canonical OWM registration message
// used by registry.Register.
func (f *NodeFixture) Sign(lnURI, tier string, ts int64) []byte {
	msg := []byte(fmt.Sprintf("owm-register|%s|%s|%s|%d", f.PubKeyHex, lnURI, tier, ts))
	return ed25519.Sign(f.PrivKey, msg)
}
