package config

import (
	"testing"
)

func TestLoadDatabaseDSNFromEnv(t *testing.T) {
	t.Setenv("OWM_DATABASE_DSN", "postgres://u:p@h:5432/db?sslmode=disable")
	t.Setenv("OWM_DEV_MODE", "true")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.DSN == "" {
		t.Fatal("expected Database.DSN from OWM_DATABASE_DSN")
	}
	if !cfg.DevMode {
		t.Fatal("expected DevMode from OWM_DEV_MODE")
	}
}

func TestLoadLightningFromEnv(t *testing.T) {
	t.Setenv("OWM_DATABASE_DSN", "postgres://u:p@h:5432/db?sslmode=disable")
	t.Setenv("OWM_DEV_MODE", "false")
	t.Setenv("OWM_LIGHTNING_LND_HOST", "lnd:10009")
	t.Setenv("OWM_LIGHTNING_READONLY_MACAROON_PATH", "/ro")
	t.Setenv("OWM_LIGHTNING_PAYMENT_MACAROON_PATH", "/pay")
	t.Setenv("OWM_LIGHTNING_SLASHING_MACAROON_PATH", "/slash")
	t.Setenv("OWM_LIGHTNING_TLS_CERT_PATH", "/tls")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lightning.LNDHost != "lnd:10009" {
		t.Fatalf("LNDHost: got %q", cfg.Lightning.LNDHost)
	}
	if cfg.Lightning.ReadonlyMacaroonPath != "/ro" {
		t.Fatalf("ReadonlyMacaroonPath: got %q", cfg.Lightning.ReadonlyMacaroonPath)
	}
}
