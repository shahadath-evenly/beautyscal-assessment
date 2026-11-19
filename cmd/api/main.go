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
	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/email"
	"github.com/real-musafir/b-scale/backend/internal/jobs"
	"github.com/real-musafir/b-scale/backend/internal/modules/orderrequest"
	"github.com/real-musafir/b-scale/backend/internal/modules/product"
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

	// One Queries value over the pool, shared by the middleware and, later,
	// by the feature services. Transactions get their own via sqlc.New(tx).
	queries := sqlc.New(pool)

	// Each module is wired explicitly: repository, then service, then
	// handler. No container, no reflection, no hidden globals.
	validator := validate.New()

	// Buyer notifications. The outbox writes the event inside the acceptance
	// transaction; the worker sends it afterwards, so the HTTP response never
	// waits on an email provider.
	outbox := email.NewOutbox(cfg.Email.MaxAttempts)

	productHandler := product.NewHandler(
		product.NewService(product.NewRepository(queries)),
		logger,
	)

	orderRequestHandler := orderrequest.NewHandler(
		orderrequest.NewService(orderrequest.NewRepository(pool, queries, outbox)),
		validator,
		logger,
	)

	router := server.NewRouter(server.Deps{
		Logger:             logger,
		Pool:               pool,
		Validator:          validator,
		Workspaces:         queries,
		Products:           productHandler,
		OrderRequests:      orderRequestHandler,
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		RequestTimeout:     cfg.RequestTimeout,
		MaxBodyBytes:       cfg.MaxBodyBytes,
	})

	// The worker runs beside the server and stops with it. Its context is the
	// signal context, so Ctrl-C ends the poll loop while each in-flight send
	// keeps its own deadline.
	var workerDone chan struct{}
	if cfg.Email.Enabled {
		sender, err := newSender(cfg, logger)
		if err != nil {
			return err
		}
		worker := email.NewWorker(queries, sender, logger, email.WorkerConfig{
			PollInterval:    cfg.Email.PollInterval,
			BatchSize:       cfg.Email.BatchSize,
			Concurrency:     cfg.Email.Concurrency,
			SendTimeout:     cfg.Email.SendTimeout,
			RetryBaseDelay:  cfg.Email.RetryBaseDelay,
			RetryMaxDelay:   cfg.Email.RetryMaxDelay,
			StaleClaimAfter: cfg.Email.StaleClaimAfter,
		})
		workerDone = make(chan struct{})
		go func() {
			defer close(workerDone)
			worker.Run(ctx)
		}()
		report.Done("Email worker running (" + sender.Name() + ")")
	} else {
		logger.Info("email worker disabled; acceptance events are still queued")
	}

	// The expiry job runs beside the server too. It takes a PostgreSQL
	// advisory lock before each sweep, so running several API processes
	// against one database stays safe.
	var expiryDone chan struct{}
	if cfg.Expiry.Enabled {
		runner := jobs.NewExpiryRunner(
			queries,
			jobs.NewAdvisoryLocker(pool, jobs.ExpiryLockKey, logger),
			logger,
			jobs.ExpiryConfig{
				Interval: cfg.Expiry.Interval,
				MaxAge:   cfg.Expiry.MaxAge,
				Timeout:  cfg.Expiry.Timeout,
			},
		)
		expiryDone = make(chan struct{})
		go func() {
			defer close(expiryDone)
			runner.Run(ctx)
		}()
		report.Done("Request expiry job running (every " + cfg.Expiry.Interval.String() +
			", expiring after " + cfg.Expiry.MaxAge.String() + ")")
	} else {
		logger.Info("request expiry job disabled; stale requests stay new")
	}

	srv := server.New(server.Config{
		Addr:              cfg.Addr(),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		// Called once the port is bound, so the address printed is real.
		OnListen:   func(addr string) { report.Listening(addr, cfg.AppEnv) },
		OnShutdown: func() { report.Done("Shutting down, draining connections") },
	}, router, logger)

	if err := srv.Run(ctx); err != nil {
		return err
	}
	// The server has drained; let the worker finish its current batch before
	// the pool closes under it.
	if workerDone != nil {
		<-workerDone
	}
	if expiryDone != nil {
		<-expiryDone
	}
	report.Done("Server stopped")
	return nil
}

// newSender picks the delivery mechanism. Brevo when it is configured, the
// logging sender otherwise: the assessment accepts console logging, and this
// keeps the outbox, the worker and the retry policy exercised on a machine
// with no credentials.
func newSender(cfg config.Config, logger *slog.Logger) (email.Sender, error) {
	if !cfg.Email.Configured() {
		logger.Info("no email provider configured; messages will be logged, not sent")
		return email.NewLogSender(logger), nil
	}
	return email.NewBrevoSender(email.BrevoConfig{
		APIKey:      cfg.Email.BrevoAPIKey,
		SenderEmail: cfg.Email.BrevoSenderEmail,
		SenderName:  cfg.Email.BrevoSenderName,
		Timeout:     cfg.Email.SendTimeout,
	})
}
