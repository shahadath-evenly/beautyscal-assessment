// Package db owns the PostgreSQL connection pool and, later, the sqlc
// generated query code.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig carries the pool settings this package needs, keeping it
// independent of the application-wide config type.
type PoolConfig struct {
	DatabaseURL    string
	MaxConns       int32
	MinConns       int32
	ConnectTimeout time.Duration
}

// NewPool parses the connection string, opens a pgx pool and verifies
// connectivity with a bounded ping so startup fails fast when the database is
// unreachable. ConnectTimeout bounds both that ping and every connection the
// pool opens later. The caller owns the returned pool and must Close it.
func NewPool(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: parse database url: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	// Bound every connection the pool opens, not just the first. Without
	// this, a connection the pool creates mid-request to serve a burst can
	// stall indefinitely and consume the whole request deadline, so the
	// caller sees a timeout (500) instead of a prompt failure.
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}
