package db

import (
	"context"
	"testing"
	"time"
)

// A stalled connection must not outlive ConnectTimeout. The address is from
// TEST-NET-3 (RFC 5737), which is reserved for documentation and never
// routable, so the dial either times out or is refused promptly — what is
// asserted is that NewPool gives up rather than hanging until the caller's
// deadline expires.
func TestNewPoolBoundsConnectTimeout(t *testing.T) {
	const connectTimeout = 500 * time.Millisecond

	start := time.Now()
	pool, err := NewPool(context.Background(), PoolConfig{
		DatabaseURL:    "postgres://user@203.0.113.1:5432/nothing?sslmode=disable",
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: connectTimeout,
	})
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("expected an error connecting to an unroutable address")
	}
	// Generous headroom: the point is that the timeout is applied at all,
	// not that the dial fails to the millisecond.
	if elapsed > 5*time.Second {
		t.Fatalf("NewPool took %v, which is far beyond the %v connect timeout", elapsed, connectTimeout)
	}
}

func TestNewPoolRejectsMalformedURL(t *testing.T) {
	pool, err := NewPool(context.Background(), PoolConfig{
		DatabaseURL:    "://not-a-url",
		MaxConns:       4,
		ConnectTimeout: time.Second,
	})
	if err == nil {
		pool.Close()
		t.Fatal("expected a parse error")
	}
}
