package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Config holds the HTTP server's own settings.
type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration

	// OnListen, when set, is called once the listener is bound and before any
	// request is served. It runs after the bind succeeds, so whatever it
	// reports is true: a port conflict fails before this point.
	OnListen func(addr string)
	// OnShutdown, when set, is called when the shutdown signal arrives and
	// before connections are drained.
	OnShutdown func()
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
	// Bound explicitly rather than through ListenAndServe, so a port conflict
	// is reported here instead of from inside the serving goroutine, and so
	// OnListen only ever runs on a successful bind.
	listener, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("server: listen on %s: %w", s.cfg.Addr, err)
	}

	s.logger.Debug("http server listening", "addr", s.cfg.Addr)
	if s.cfg.OnListen != nil {
		s.cfg.OnListen(s.cfg.Addr)
	}

	errCh := make(chan error, 1)

	go func() {
		if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("server: serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Debug("shutdown signal received, draining connections", "timeout", s.cfg.ShutdownTimeout)
		if s.cfg.OnShutdown != nil {
			s.cfg.OnShutdown()
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()

		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: graceful shutdown: %w", err)
		}
		s.logger.Debug("http server stopped cleanly")
		return nil
	}
}
