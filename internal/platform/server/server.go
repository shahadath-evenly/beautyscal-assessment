package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Config holds the HTTP server's own settings.
type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

// Server owns the lifecycle of the HTTP listener.
type Server struct {
	http   *http.Server
	logger *slog.Logger
	cfg    Config
}

// New returns a Server for the given handler.
func New(cfg Config, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              cfg.Addr,
			Handler:           handler,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		},
		logger: logger,
		cfg:    cfg,
	}
}

// Run serves until ctx is cancelled, then drains in-flight requests within the
// configured shutdown timeout.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("http server listening", "addr", s.cfg.Addr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("server: listen: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutdown signal received, draining connections", "timeout", s.cfg.ShutdownTimeout)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()

		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: graceful shutdown: %w", err)
		}
		s.logger.Info("http server stopped cleanly")
		return nil
	}
}
