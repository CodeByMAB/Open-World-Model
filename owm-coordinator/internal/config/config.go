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
	DevMode   bool `mapstructure:"dev_mode"` // when true, Lightning may use mock client; LND/macaroon not required
	Server    ServerConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	Lightning LightningConfig
	S3        S3Config
	FL        FLConfig
	Stake     StakeConfig
	Tor       TorConfig
	Observer  ObserverConfig
	Log       LogConfig
}

type ServerConfig struct {
	GRPCAddr        string        `mapstructure:"grpc_addr"` // e.g. ":9000"
	HTTPAddr        string        `mapstructure:"http_addr"` // e.g. ":9001" (internal HTTP for health/metrics)
	TLSCertFile     string        `mapstructure:"tls_cert_file"`
	TLSKeyFile      string        `mapstructure:"tls_key_file"`
	CACertFile      string        `mapstructure:"ca_cert_file"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	DSN           string `mapstructure:"dsn"` // postgres connection string
	MaxOpenConns  int    `mapstructure:"max_open_conns"`
	MaxIdleConns  int    `mapstructure:"max_idle_conns"`
	MigrationsDir string `mapstructure:"migrations_dir"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type LightningConfig struct {
	Backend              string    `mapstructure:"backend"` // "lnd" | "cln", default "lnd"
	LNDHost              string    `mapstructure:"lnd_host"`
	PaymentMacaroonPath  string    `mapstructure:"payment_macaroon_path"`
	ReadonlyMacaroonPath string    `mapstructure:"readonly_macaroon_path"`
	SlashingMacaroonPath string    `mapstructure:"slashing_macaroon_path"`
	TLSCertPath          string    `mapstructure:"tls_cert_path"`
	CLN                  CLNConfig `mapstructure:"cln"`
}

// CLNConfig holds Core Lightning CLNRest settings.
type CLNConfig struct {
	BaseURL string `mapstructure:"base_url"` // e.g. "https://127.0.0.1:3010"
	APIKey  string `mapstructure:"api_key"`  // CLN rune (Rune HTTP header)
}

type S3Config struct {
	Endpoint  string `mapstructure:"endpoint"`
	Bucket    string `mapstructure:"bucket"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Region    string `mapstructure:"region"`
}

type FLConfig struct {
	RoundIntervalMinutes   int     `mapstructure:"round_interval_minutes"`
	MinParticipants        int     `mapstructure:"min_participants"`
	GradientL2ClipNorm     float64 `mapstructure:"gradient_l2_clip_norm"`
	AnomalyStdDevThreshold float64 `mapstructure:"anomaly_std_dev_threshold"`
	TopKSparsificationPct  float64 `mapstructure:"top_k_sparsification_pct"` // e.g. 0.10 = top 10%
}

type StakeConfig struct {
	TierMinimumSats          map[string]int64 `mapstructure:"tier_minimum_sats"`
	VerifyIntervalHours      int              `mapstructure:"verify_interval_hours"`
	DegradedGracePeriodHours int              `mapstructure:"degraded_grace_period_hours"`
	SlashCooldownDays        int              `mapstructure:"slash_cooldown_days"`
	T1AutoSlashSignals       int              `mapstructure:"t1_auto_slash_signals"`
	T2T3MaintainerAcks       int              `mapstructure:"t2t3_maintainer_acks"`
}

// TorConfig controls the optional Tor dual-transport layer (SRS-NET-01, ADR-005).
// When Enabled is true the coordinator binds a second gRPC listener on
// LocalBindAddr and expects the Tor daemon to forward the hidden service port
// to that address. Data-plane traffic (FL gradients, model weights, Stratum v2)
// never goes through Tor regardless of this setting.
type TorConfig struct {
	Enabled          bool   `mapstructure:"enabled"` // Tor hidden service listener and SOCKS5 dialer
	LocalBindAddr    string `mapstructure:"local_bind_addr"`
	SOCKS5Addr       string `mapstructure:"socks5_addr"`
	HiddenServiceDir string `mapstructure:"hidden_service_dir"`
}

// ObserverConfig controls the Observer Protocol v0.1 receipt submission.
// When Enabled is true, the coordinator signs and submits payment receipts
// to the Observer Registry (api.observerprotocol.org) after each successful
// Lightning payment. SigningKeyPath must point to an Ed25519 private key.
type ObserverConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	APIEndpoint    string `mapstructure:"api_endpoint"`
	SigningKeyPath string `mapstructure:"signing_key_path"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`  // "debug" | "info" | "warn" | "error"
	Format string `mapstructure:"format"` // "json" | "console"
}

// Load reads configuration from environment variables (OWM_* prefix) and
// an optional config file path. Environment variables take precedence.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.grpc_addr", ":9000")
	v.SetDefault("server.http_addr", ":9001")
	v.SetDefault("server.shutdown_timeout", "30s")
	// Empty defaults so AutomaticEnv keys participate in Unmarshal (viper AllKeys).
	v.SetDefault("server.tls_cert_file", "")
	v.SetDefault("server.tls_key_file", "")
	v.SetDefault("server.ca_cert_file", "")
	v.SetDefault("database.dsn", "")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.migrations_dir", "migrations")
	v.SetDefault("redis.addr", "")
	v.SetDefault("redis.password", "")
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
	v.SetDefault("observer.enabled", false)
	v.SetDefault("observer.api_endpoint", "https://api.observerprotocol.org")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("dev_mode", false)
	v.SetDefault("lightning.backend", "lnd")
	v.SetDefault("lightning.lnd_host", "")
	v.SetDefault("lightning.readonly_macaroon_path", "")
	v.SetDefault("lightning.payment_macaroon_path", "")
	v.SetDefault("lightning.slashing_macaroon_path", "")
	v.SetDefault("lightning.tls_cert_path", "")
	v.SetDefault("lightning.cln.base_url", "")
	v.SetDefault("lightning.cln.api_key", "")
	v.SetDefault("s3.endpoint", "")
	v.SetDefault("s3.bucket", "")
	v.SetDefault("s3.access_key", "")
	v.SetDefault("s3.secret_key", "")
	v.SetDefault("s3.region", "")
	v.SetDefault("observer.signing_key_path", "")

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
	backend := strings.ToLower(strings.TrimSpace(c.Lightning.Backend))
	if backend == "" {
		backend = "lnd"
	}
	c.Lightning.Backend = backend

	// In dev mode, Lightning credentials are optional (mock client is used).
	if !c.DevMode {
		switch backend {
		case "lnd":
			if c.Lightning.LNDHost == "" {
				return fmt.Errorf("lightning.lnd_host (OWM_LIGHTNING_LND_HOST) is required for backend lnd (set OWM_DEV_MODE=true to use mock)")
			}
			if c.Lightning.TLSCertPath == "" {
				return fmt.Errorf("lightning.tls_cert_path (OWM_LIGHTNING_TLS_CERT_PATH) is required for backend lnd when dev_mode is false: LND macaroon auth must use TLS to the node")
			}
			if c.Lightning.PaymentMacaroonPath == "" {
				return fmt.Errorf("lightning.payment_macaroon_path is required for backend lnd")
			}
			if c.Lightning.ReadonlyMacaroonPath == "" {
				return fmt.Errorf("lightning.readonly_macaroon_path is required for backend lnd")
			}
			if c.Lightning.SlashingMacaroonPath == "" {
				return fmt.Errorf("lightning.slashing_macaroon_path is required for backend lnd")
			}
		case "cln":
			if c.Lightning.CLN.BaseURL == "" {
				return fmt.Errorf("lightning.cln.base_url is required for backend cln")
			}
			if c.Lightning.CLN.APIKey == "" {
				return fmt.Errorf("lightning.cln.api_key is required for backend cln")
			}
		default:
			return fmt.Errorf("lightning.backend must be 'lnd' or 'cln', got %q", c.Lightning.Backend)
		}
	}
	// mTLS: when TLS is used and dev_mode is false, CA cert is required for client verification.
	if c.Server.TLSCertFile != "" && !c.DevMode && c.Server.CACertFile == "" {
		return fmt.Errorf("server.ca_cert_file is required when TLS is enabled and dev_mode is false (mTLS enforcement)")
	}
	// Observer: when enabled, signing key is required for receipt signatures.
	if c.Observer.Enabled && c.Observer.SigningKeyPath == "" {
		return fmt.Errorf("observer.signing_key_path is required when observer.enabled is true")
	}
	if c.Observer.Enabled && strings.TrimSpace(c.Observer.APIEndpoint) == "" {
		return fmt.Errorf("observer.api_endpoint is required when observer.enabled is true")
	}
	return nil
}
