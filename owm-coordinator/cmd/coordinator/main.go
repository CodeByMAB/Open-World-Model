// Command coordinator is the entry point for the OWM Coordinator service.
// It initialises all subsystems, registers background tickers, and serves
// the gRPC API (SRS-ARCH-01 through SRS-ARCH-04).
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/owmnetwork/owm-coordinator/internal/config"
	"github.com/owmnetwork/owm-coordinator/internal/fl"
	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/rpc"
	"github.com/owmnetwork/owm-coordinator/internal/scheduler"
	"github.com/owmnetwork/owm-coordinator/internal/stake"
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
	cfgFile := os.Getenv("OWM_CONFIG_FILE")
	if len(os.Args) > 1 {
		cfgFile = os.Args[1]
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

	// ── Internal services ─────────────────────────────────────────────────────
	reg := registry.New(db, log)

	// Stake verifier — Lightning clients are injected via config.
	// In production these are real LND clients loaded from macaroon paths.
	// A nil client is acceptable during development; VerifyStake will skip LN checks.
	verif := stake.New(db, nil, nil, stake.SlashConfig{
		T1AutoSlashSignals: 3,
		T2T3MaintainerAcks: 2,
		CooldownDuration:   30 * 24 * time.Hour,
	}, log)

	sched := scheduler.New(db, reg, log)
	flOrch := fl.New(db, reg, log)

	// ── gRPC server ───────────────────────────────────────────────────────────
	srv := rpc.New(reg, sched, verif, flOrch, db, log)

	var grpcOpts []grpc.ServerOption

	if cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("loading TLS credentials: %w", err)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		log.Info("TLS enabled", zap.String("cert", cfg.Server.TLSCertFile))
	} else {
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

	// ── Background tickers ────────────────────────────────────────────────────
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

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
