package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpiryLockKey identifies the request-expiry advisory lock.
//
// PostgreSQL advisory locks share one global namespace keyed by a bare
// int64, so the number itself is arbitrary; what matters is that nothing
// else in the system picks the same one. It is a constant rather than a
// setting because two processes must agree on it to exclude each other.
const ExpiryLockKey int64 = 8_245_119_001

// AdvisoryLocker takes a PostgreSQL session-level advisory lock.
//
// This is what keeps two API processes from sweeping at the same time. A
// session-level lock, rather than a transaction-level one, because the sweep
// is a single statement and the lock has to be held across the call rather
// than inside it. If a process dies mid-sweep the session ends and the lock
// is released by the server, so a crash cannot wedge the job permanently.
type AdvisoryLocker struct {
	pool   *pgxpool.Pool
	key    int64
	logger *slog.Logger
}

// NewAdvisoryLocker wires a locker over the shared pool.
func NewAdvisoryLocker(pool *pgxpool.Pool, key int64, logger *slog.Logger) *AdvisoryLocker {
	return &AdvisoryLocker{pool: pool, key: key, logger: logger}
}

// TryLock acquires the lock without waiting. It returns ok=false when another
// session holds it.
//
// The lock is bound to one connection, so that connection is held for the
// duration and released by the returned function. Acquiring and releasing on
// the same session is not optional: pg_advisory_unlock on a different
// connection is a no-op and would leak the lock.
func (l *AdvisoryLocker) TryLock(ctx context.Context) (func(), bool, error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}

	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", l.key).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}

	release := func() {
		// WithoutCancel: a shutdown mid-sweep must still release the lock
		// rather than leave it for the session teardown to clean up.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if _, err := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", l.key); err != nil {
			// Not fatal: releasing the connection ends the session, and the
			// server drops the lock with it.
			l.logger.Warn("request expiry job: releasing the advisory lock failed", "error", err)
		}
		conn.Release()
	}
	return release, true, nil
}

// NoLocker runs every sweep unconditionally. It exists for single-process
// setups and for tests; the API wires AdvisoryLocker.
type NoLocker struct{}

// TryLock always succeeds.
func (NoLocker) TryLock(context.Context) (func(), bool, error) {
	return func() {}, true, nil
}
