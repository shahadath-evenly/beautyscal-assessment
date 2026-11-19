// Command api runs the BeautyScale Brand Order Requests HTTP API.
//
// Startup is explicit and ordered: configuration, logger, database pool,
// router, server. Every dependency is constructed here and passed down, so the
// wiring of the application is readable in one place.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/real-musafir/b-scale/backend/internal/config"
	"github.com/real-musafir/b-scale/backend/internal/db"
	"github.com/real-musafir/b-scale/backend/internal/platform/logging"
	"github.com/real-musafir/b-scale/backend/internal/platform/server"
	"github.com/real-musafir/b-scale/backend/internal/platform/startup"
	"github.com/real-musafir/b-scale/backend/internal/platform/validate"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet if configuration failed, so report the
		// fatal error on stderr and exit non-zero.
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)

	// Text format means a person is reading the terminal, so startup
	// milestones are printed readably. JSON format means a log collector is
	// reading them, so they go through the logger instead.
	report := startup.New(logger, cfg.LogFormat == "text")
	report.EnvLoaded(cfg.EnvFile, cfg.EnvInjected)

	// Cancelled on SIGINT/SIGTERM, which drives graceful shutdown below.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, db.PoolConfig{
		DatabaseURL:    cfg.DatabaseURL,
		MaxConns:       cfg.DBMaxConns,
		MinConns:       cfg.DBMinConns,
		ConnectTimeout: cfg.DBConnectTimeout,
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	report.DatabaseConnected()
	logger.Debug("database pool ready", "maxConns", cfg.DBMaxConns)

	router := server.NewRouter(server.Deps{
		Logger:             logger,
		Pool:               pool,
		Validator:          validate.New(),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		RequestTimeout:     cfg.RequestTimeout,
	})

	srv := server.New(server.Config{
		Addr:              cfg.Addr(),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		// Called once the port is bound, so the address printed is real.
		OnListen:   func(addr string) { report.Listening(addr, cfg.AppEnv) },
		OnShutdown: func() { report.Done("Shutting down, draining connections") },
	}, router, logger)

	if err := srv.Run(ctx); err != nil {
		return err
	}
	report.Done("Server stopped")
	return nil
}
