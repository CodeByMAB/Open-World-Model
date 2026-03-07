// Command coordinator is the entry point for the OWM Coordinator service.
// It initialises all subsystems, registers background tickers, and serves
// the gRPC API (SRS-ARCH-01 through SRS-ARCH-04).
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/owmnetwork/owm-coordinator/internal/config"
	"github.com/owmnetwork/owm-coordinator/internal/fl"
	_ "github.com/owmnetwork/owm-coordinator/internal/metrics"
	"github.com/owmnetwork/owm-coordinator/internal/lightning"
	"github.com/owmnetwork/owm-coordinator/internal/lightning/mock"
	"github.com/owmnetwork/owm-coordinator/internal/observer"
	"github.com/owmnetwork/owm-coordinator/internal/payment"
	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/rpc"
	"github.com/owmnetwork/owm-coordinator/internal/scheduler"
	"github.com/owmnetwork/owm-coordinator/internal/stake"
	"github.com/owmnetwork/owm-coordinator/internal/storage"
	coordinatorv1 "github.com/owmnetwork/owm-coordinator/proto/coordinator/v1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "coordinator: fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// ── Config ────────────────────────────────────────────────────────────────
	// Optionally pass a config file path via OWM_CONFIG_FILE or as first arg.
	// Support --migrate-only flag; remaining argument (if any) is config path.
	cfgFile := os.Getenv("OWM_CONFIG_FILE")
	var migrateOnly bool
	for _, a := range os.Args[1:] {
		if a == "--migrate-only" {
			migrateOnly = true
		} else {
			cfgFile = a
		}
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// ── Logger ────────────────────────────────────────────────────────────────
	log, err := buildLogger(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return fmt.Errorf("building logger: %w", err)
	}
	defer log.Sync() //nolint:errcheck

	log.Info("starting owm-coordinator", zap.String("version", "0.1.0"))

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := pgxpool.New(context.Background(), cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := db.Ping(ctx); err != nil {
		cancel()
		return fmt.Errorf("database ping: %w", err)
	}
	cancel()
	log.Info("database connection established")

	// ── Migrations ──────────────────────────────────────────────────────────
	migrationsPath := cfg.Database.MigrationsDir
	if migrationsPath == "" {
		migrationsPath = "migrations"
	}
	m, err := migrate.New("file://"+migrationsPath, cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer m.Close()
	migrateErr := m.Up()
	if migrateErr != nil && migrateErr != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", migrateErr)
	}
	if migrateErr == migrate.ErrNoChange {
		log.Info("migrations: no change")
	} else {
		log.Info("migrations applied successfully")
	}
	if migrateOnly {
		log.Info("migrate-only mode: exiting")
		return nil
	}

	// ── Internal services ─────────────────────────────────────────────────────
	reg := registry.New(db, log)

	// Lightning clients: mock in dev mode, real LND/CLN in production (T2).
	var lnReadonly, lnPayment, lnSlash lightning.Client
	if cfg.DevMode {
		mockClient := mock.New()
		lnReadonly = mockClient
		lnPayment = mockClient
		lnSlash = mockClient
		log.Warn("dev_mode active — using mock Lightning client, no real payments or slashing")
	} else {
		if cfg.Lightning.Backend == "cln" {
			clnReadonly := lightning.NewCLNClient(cfg.Lightning.CLN.BaseURL, cfg.Lightning.CLN.APIKey)
			clnPayment := lightning.NewCLNClient(cfg.Lightning.CLN.BaseURL, cfg.Lightning.CLN.APIKey)
			clnSlash := lightning.NewCLNClient(cfg.Lightning.CLN.BaseURL, cfg.Lightning.CLN.APIKey)
			lnReadonly = clnReadonly
			lnPayment = clnPayment
			lnSlash = clnSlash
		} else {
			readonlyMac, err := os.ReadFile(cfg.Lightning.ReadonlyMacaroonPath)
			if err != nil {
				return fmt.Errorf("reading readonly macaroon: %w", err)
			}
			paymentMac, err := os.ReadFile(cfg.Lightning.PaymentMacaroonPath)
			if err != nil {
				return fmt.Errorf("reading payment macaroon: %w", err)
			}
			slashingMac, err := os.ReadFile(cfg.Lightning.SlashingMacaroonPath)
			if err != nil {
				return fmt.Errorf("reading slashing macaroon: %w", err)
			}
			lndReadonly, err := lightning.NewLNDClient(cfg.Lightning.LNDHost, cfg.Lightning.TLSCertPath, readonlyMac)
			if err != nil {
				return fmt.Errorf("LND readonly client: %w", err)
			}
			lndPayment, err := lightning.NewLNDClient(cfg.Lightning.LNDHost, cfg.Lightning.TLSCertPath, paymentMac)
			if err != nil {
				return fmt.Errorf("LND payment client: %w", err)
			}
			lndSlash, err := lightning.NewLNDClient(cfg.Lightning.LNDHost, cfg.Lightning.TLSCertPath, slashingMac)
			if err != nil {
				return fmt.Errorf("LND slashing client: %w", err)
			}
			lnReadonly = lndReadonly
			lnPayment = lndPayment
			lnSlash = lndSlash
		}
	}

	verif := stake.New(db, lnReadonly, lnSlash, stake.SlashConfig{
		T1AutoSlashSignals: 3,
		T2T3MaintainerAcks: 2,
		CooldownDuration:   30 * 24 * time.Hour,
	}, log)

	var rdb *redis.Client
	if cfg.Redis.Addr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
			PoolSize: 1020,
		})
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			log.Warn("redis ping failed; task streaming may be disabled", zap.Error(err))
		}
	}

	sched := scheduler.New(db, reg, rdb, log)
	var s3Client *storage.S3Client
	if cfg.S3.Bucket != "" {
		var err error
		s3Client, err = storage.NewS3Client(cfg.S3)
		if err != nil {
			return fmt.Errorf("S3 client: %w", err)
		}
	}
	flOrch := fl.NewWithConfig(db, reg, s3Client, &fl.OrchestratorConfig{
		GradientL2ClipNorm:     cfg.FL.GradientL2ClipNorm,
		AnomalyStdDevThreshold: cfg.FL.AnomalyStdDevThreshold,
	}, log)

	var obsClient *observer.Client
	if cfg.Observer.Enabled {
		keyData, err := os.ReadFile(cfg.Observer.SigningKeyPath)
		if err != nil {
			return fmt.Errorf("observer signing key: %w", err)
		}
		obsClient, err = observer.NewClient(observer.ClientConfig{
			APIEndpoint:       cfg.Observer.APIEndpoint,
			SigningKeyPEMOrHex: keyData,
			Log:               log,
		})
		if err != nil {
			return fmt.Errorf("observer client: %w", err)
		}
		info, err := lnReadonly.GetInfo(context.Background())
		if err != nil {
			return fmt.Errorf("observer: get treasury pubkey (GetInfo): %w", err)
		}
		senderPubkeyHash := observer.PubkeyHashHex(info.PubkeyHex)
		if senderPubkeyHash == "" {
			return fmt.Errorf("observer: treasury pubkey from GetInfo produced empty hash (pubkey_hex length=%d); cannot set sender_public_key_hash for receipt submission", len(info.PubkeyHex))
		}
		obsClient.SetSenderPubkeyHash(senderPubkeyHash)
		log.Info("observer protocol enabled", zap.String("api", cfg.Observer.APIEndpoint))
	}
	disp := payment.New(lnPayment, db, rdb, log, obsClient)

	// ── gRPC server ───────────────────────────────────────────────────────────
	srv := rpc.New(reg, sched, verif, flOrch, disp, rdb, db, log)

	var grpcOpts []grpc.ServerOption
	if cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != "" && !cfg.DevMode {
		caPEM, err := os.ReadFile(cfg.Server.CACertFile)
		if err != nil {
			return fmt.Errorf("reading CA cert: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return fmt.Errorf("parsing CA cert failed")
		}
		serverCert, err := tls.LoadX509KeyPair(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("loading server TLS: %w", err)
		}
		tlsCfg := &tls.Config{
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    caPool,
			Certificates: []tls.Certificate{serverCert},
			MinVersion:   tls.VersionTLS13,
		}
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		log.Info("mTLS enabled", zap.String("cert", cfg.Server.TLSCertFile), zap.String("ca", cfg.Server.CACertFile))
	} else if cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != "" && cfg.DevMode {
		creds, err := credentials.NewServerTLSFromFile(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("loading TLS credentials: %w", err)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		log.Info("TLS enabled (dev mode — client cert not required)", zap.String("cert", cfg.Server.TLSCertFile))
	} else if cfg.Server.TLSCertFile == "" {
		log.Warn("TLS not configured — running in plaintext mode (not suitable for production)")
	}

	grpcSrv := grpc.NewServer(grpcOpts...)
	coordinatorv1.RegisterCoordinatorServiceServer(grpcSrv, srv)

	// ── Clearnet listener ─────────────────────────────────────────────────────
	addr := cfg.Server.GRPCAddr
	if addr == "" {
		addr = ":9000"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	log.Info("gRPC server listening (clearnet)", zap.String("addr", addr))

	// ── Metrics HTTP server ───────────────────────────────────────────────────
	httpAddr := cfg.Server.HTTPAddr
	if httpAddr == "" {
		httpAddr = ":9001"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	httpSrv := &http.Server{Addr: httpAddr, Handler: mux}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics HTTP server failed", zap.Error(err))
		}
	}()

	// ── Tor hidden service listener (optional, SRS-NET-01, SRS-COORD-07) ─────
	if cfg.Tor.Enabled {
		torAddr := cfg.Tor.LocalBindAddr
		if torAddr == "" {
			torAddr = "127.0.0.1:9002"
		}
		torLis, err := net.Listen("tcp", torAddr)
		if err != nil {
			return fmt.Errorf("listening on Tor local bind addr %s: %w", torAddr, err)
		}
		log.Info("gRPC server listening (Tor local bind)", zap.String("addr", torAddr))

		// Read the .onion hostname from the Tor HiddenServiceDir (SRS-COORD-09).
		onionHostname := readOnionHostname(cfg.Tor.HiddenServiceDir, log)
		if onionHostname != "" {
			log.Info("Tor hidden service active",
				zap.String("onion_address", onionHostname),
				zap.String("socks5_addr", cfg.Tor.SOCKS5Addr),
			)
		}

		// Serve the same gRPC server on the Tor-forwarded local port.
		go func() {
			if err := grpcSrv.Serve(torLis); err != nil {
				log.Error("Tor gRPC listener stopped", zap.Error(err))
			}
		}()
	}

	// ── Background tickers and dispatcher ───────────────────────────────────────
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	disp.Start(bgCtx)

	go runTicker(bgCtx, log, "requeue-timed-out", 60*time.Second, func(ctx context.Context) {
		n, err := sched.RequeueTimedOut(ctx)
		if err != nil {
			log.Error("requeue timed-out tasks", zap.Error(err))
		} else if n > 0 {
			log.Info("timed-out tasks requeued", zap.Int("count", n))
		}
	})

	go runTicker(bgCtx, log, "stake-reverify", 6*time.Hour, func(ctx context.Context) {
		if err := verif.VerifyAllActive(ctx); err != nil {
			log.Error("periodic stake reverification", zap.Error(err))
		}
	})

	go runTicker(bgCtx, log, "fl-expire-rounds", 5*time.Minute, func(ctx context.Context) {
		n, err := flOrch.CloseExpired(ctx)
		if err != nil {
			log.Error("closing expired FL rounds", zap.Error(err))
		} else if n > 0 {
			log.Info("expired FL rounds closed", zap.Int("count", n))
		}
	})

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			errCh <- fmt.Errorf("gRPC serve: %w", err)
		}
	}()

	select {
	case sig := <-quit:
		log.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-errCh:
		return err
	}

	bgCancel()
	_ = httpSrv.Shutdown(context.Background())
	grpcSrv.GracefulStop()
	log.Info("coordinator stopped cleanly")
	return nil
}

// readOnionHostname reads the Tor hidden service hostname from
// HiddenServiceDir/hostname and returns it trimmed, or "" on any error.
func readOnionHostname(hiddenServiceDir string, log *zap.Logger) string {
	hostFile := filepath.Join(hiddenServiceDir, "hostname")
	data, err := os.ReadFile(hostFile)
	if err != nil {
		log.Warn("could not read Tor hidden service hostname",
			zap.String("path", hostFile),
			zap.Error(err),
		)
		return ""
	}
	return strings.TrimSpace(string(data))
}

// runTicker fires fn on the given interval until ctx is cancelled.
func runTicker(ctx context.Context, log *zap.Logger, name string, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Debug("ticker started", zap.String("name", name), zap.Duration("interval", interval))
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

// buildLogger constructs a zap.Logger from the given level and format strings.
func buildLogger(level, format string) (*zap.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = zapcore.InfoLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var enc zapcore.Encoder
	if format == "console" {
		enc = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		enc = zapcore.NewJSONEncoder(encoderCfg)
	}

	core := zapcore.NewCore(enc, zapcore.AddSync(os.Stdout), lvl)
	return zap.New(core, zap.AddCaller()), nil
}
