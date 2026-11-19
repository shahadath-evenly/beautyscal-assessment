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
	"github.com/real-musafir/b-scale/backend/internal/platform/startup"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)
	report := startup.New(logger, cfg.LogFormat == "text")
	report.EnvLoaded(cfg.EnvFile, cfg.EnvInjected)

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

	report.Done("Seed data is implemented in a later phase")
	return nil
}
