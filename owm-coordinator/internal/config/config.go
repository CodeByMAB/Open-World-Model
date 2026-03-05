// Package config loads and validates coordinator configuration from environment
// variables and an optional TOML/YAML config file via Viper.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all coordinator runtime configuration.
type Config struct {
	DevMode   bool // when true, Lightning may use mock client; LND/macaroon not required
	Server    ServerConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	Lightning LightningConfig
	FL        FLConfig
	Stake     StakeConfig
	Tor       TorConfig
	Log       LogConfig
}

type ServerConfig struct {
	GRPCAddr    string        // e.g. ":9000"
	HTTPAddr    string        // e.g. ":9001" (internal HTTP for health/metrics)
	TLSCertFile string        // mTLS server cert
	TLSKeyFile  string        // mTLS server key
	CACertFile  string        // CA cert for verifying node certs
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	DSN          string // postgres connection string
	MaxOpenConns int
	MaxIdleConns int
	MigrationsDir string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type LightningConfig struct {
	LNDHost            string // e.g. "localhost:10009"
	PaymentMacaroonPath string
	ReadonlyMacaroonPath string
	SlashingMacaroonPath string
	TLSCertPath         string
}

type FLConfig struct {
	RoundIntervalMinutes int
	MinParticipants      int
	GradientL2ClipNorm   float64
	AnomalyStdDevThreshold float64
	TopKSparsificationPct  float64 // e.g. 0.10 = top 10%
}

type StakeConfig struct {
	TierMinimumSats map[string]int64 // "t1" -> 100000, "t2" -> 500000, "t3" -> 2000000
	VerifyIntervalHours int
	DegradedGracePeriodHours int
	SlashCooldownDays  int
	T1AutoSlashSignals int // signals before automated slash for T1
	T2T3MaintainerAcks int // maintainer acks required for T2/T3 slash
}

// TorConfig controls the optional Tor dual-transport layer (SRS-NET-01, ADR-005).
// When Enabled is true the coordinator binds a second gRPC listener on
// LocalBindAddr and expects the Tor daemon to forward the hidden service port
// to that address. Data-plane traffic (FL gradients, model weights, Stratum v2)
// never goes through Tor regardless of this setting.
type TorConfig struct {
	// Enabled activates the Tor hidden service listener and SOCKS5 dialer.
	Enabled bool
	// LocalBindAddr is the address the coordinator listens on for forwarded
	// Tor connections. The Tor daemon maps .onion:9000 → LocalBindAddr.
	// Default: "127.0.0.1:9002"
	LocalBindAddr string
	// SOCKS5Addr is the outbound Tor SOCKS5 proxy address used when the
	// coordinator needs to dial .onion endpoints (e.g. LND over Tor).
	// Default: "127.0.0.1:9050"
	SOCKS5Addr string
	// HiddenServiceDir is the path to the Tor HiddenServiceDir. The coordinator
	// reads HiddenServiceDir/hostname at startup to log the .onion address.
	// Default: "/var/lib/tor/owm-coordinator"
	HiddenServiceDir string
}

type LogConfig struct {
	Level  string // "debug" | "info" | "warn" | "error"
	Format string // "json" | "console"
}

// Load reads configuration from environment variables (OWM_* prefix) and
// an optional config file path. Environment variables take precedence.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.grpc_addr", ":9000")
	v.SetDefault("server.http_addr", ":9001")
	v.SetDefault("server.shutdown_timeout", "30s")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.migrations_dir", "migrations")
	v.SetDefault("redis.db", 0)
	v.SetDefault("fl.round_interval_minutes", 60)
	v.SetDefault("fl.min_participants", 3)
	v.SetDefault("fl.gradient_l2_clip_norm", 1.0)
	v.SetDefault("fl.anomaly_std_dev_threshold", 3.0)
	v.SetDefault("fl.top_k_sparsification_pct", 0.10)
	v.SetDefault("stake.tier_minimum_sats.t1", 100000)
	v.SetDefault("stake.tier_minimum_sats.t2", 500000)
	v.SetDefault("stake.tier_minimum_sats.t3", 2000000)
	v.SetDefault("stake.verify_interval_hours", 6)
	v.SetDefault("stake.degraded_grace_period_hours", 24)
	v.SetDefault("stake.slash_cooldown_days", 30)
	v.SetDefault("stake.t1_auto_slash_signals", 3)
	v.SetDefault("stake.t2t3_maintainer_acks", 2)
	v.SetDefault("tor.enabled", false)
	v.SetDefault("tor.local_bind_addr", "127.0.0.1:9002")
	v.SetDefault("tor.socks5_addr", "127.0.0.1:9050")
	v.SetDefault("tor.hidden_service_dir", "/var/lib/tor/owm-coordinator")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("dev_mode", false)

	// Environment variable binding (OWM_SERVER_GRPC_ADDR, etc.)
	v.SetEnvPrefix("OWM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Optional config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn (OWM_DATABASE_DSN) is required")
	}
	// In dev mode, Lightning credentials are optional (mock client is used).
	if !c.DevMode {
		if c.Lightning.LNDHost == "" {
			return fmt.Errorf("lightning.lnd_host (OWM_LIGHTNING_LND_HOST) is required (set OWM_DEV_MODE=true to use mock)")
		}
		if c.Lightning.PaymentMacaroonPath == "" {
			return fmt.Errorf("lightning.payment_macaroon_path is required")
		}
		if c.Lightning.ReadonlyMacaroonPath == "" {
			return fmt.Errorf("lightning.readonly_macaroon_path is required")
		}
		if c.Lightning.SlashingMacaroonPath == "" {
			return fmt.Errorf("lightning.slashing_macaroon_path is required")
		}
	}
	return nil
}
