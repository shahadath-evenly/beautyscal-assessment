// Package jobs holds the small periodic tasks that run beside the API.
//
// There is no job framework here, and deliberately so: the assessment needs
// one task on a timer, and a timer plus a PostgreSQL advisory lock expresses
// that in less code than configuring a scheduler would take. The same two
// pieces also give the two properties that matter — a run never overlaps
// itself, and two API processes never sweep at the same time.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// ExpiryStore is the slice of the generated query set this job uses.
// Declaring it here, rather than taking *sqlc.Queries, keeps the runner
// testable with a stub and states exactly what it touches.
type ExpiryStore interface {
	ExpireStaleOrderRequests(ctx context.Context, olderThan time.Time) ([]sqlc.OrderRequest, error)
}

// Locker guards one sweep. TryLock reports ok=false when somebody else holds
// the lock, which is a normal outcome, not an error: the other holder is
// doing the work.
//
// The returned release is non-nil only when ok is true, and is safe to call
// exactly once.
type Locker interface {
	TryLock(ctx context.Context) (release func(), ok bool, err error)
}

// ExpiryConfig tunes the sweep.
type ExpiryConfig struct {
	// Interval is the gap between the end of one sweep and the start of the
	// next. Measuring from the end, rather than on a fixed schedule, is what
	// stops a slow sweep from being followed immediately by another.
	Interval time.Duration
	// MaxAge is how long a request may sit in 'new' before it expires. The
	// business rule is seven days.
	MaxAge time.Duration
	// Timeout bounds a single sweep, so a stalled statement cannot hold the
	// advisory lock until the process restarts.
	Timeout time.Duration
}

// withDefaults fills anything the caller left unset.
func (c ExpiryConfig) withDefaults() ExpiryConfig {
	if c.Interval <= 0 {
		c.Interval = time.Hour
	}
	if c.MaxAge <= 0 {
		c.MaxAge = 7 * 24 * time.Hour
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	return c
}

// ExpiryRunner expires order requests left in 'new' for too long.
type ExpiryRunner struct {
	store  ExpiryStore
	lock   Locker
	logger *slog.Logger
	cfg    ExpiryConfig
	// now is swappable so a test can place the cutoff precisely.
	now func() time.Time
}

// NewExpiryRunner wires the job. Dependencies are explicit, as everywhere
// else: both the store and the lock are interfaces, so tests substitute them.
func NewExpiryRunner(store ExpiryStore, lock Locker, logger *slog.Logger, cfg ExpiryConfig) *ExpiryRunner {
	return &ExpiryRunner{
		store:  store,
		lock:   lock,
		logger: logger,
		cfg:    cfg.withDefaults(),
		now:    time.Now,
	}
}

// Run sweeps on a timer until ctx is cancelled.
//
// The loop is sequential: RunOnce is called inline, never in a goroutine, and
// the timer is reset only once it returns. One process therefore cannot run
// two sweeps at once however slow a sweep becomes, and the advisory lock
// inside RunOnce extends that guarantee across processes.
func (r *ExpiryRunner) Run(ctx context.Context) {
	r.logger.Info("request expiry job started",
		"interval", r.cfg.Interval.String(),
		"maxAge", r.cfg.MaxAge.String(),
	)
	defer r.logger.Info("request expiry job stopped")

	// Fires immediately, so a process that has been down past the window
	// catches up at startup instead of waiting out a whole interval.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if _, err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			// A failed sweep is not fatal: the next one covers the same rows,
			// because the cutoff is recomputed from the clock each time.
			r.logger.Error("request expiry job: sweep failed", "error", err)
		}

		timer.Reset(r.cfg.Interval)
	}
}

// RunOnce performs a single sweep and reports how many requests it expired.
// It is exported so a test, or an operator, can drive exactly one cycle.
func (r *ExpiryRunner) RunOnce(ctx context.Context) (int, error) {
	release, ok, err := r.lock.TryLock(ctx)
	if err != nil {
		return 0, err
	}
	if !ok {
		// Another process is sweeping. Skipping is the correct outcome, so
		// this is not an error and not a warning.
		r.logger.Debug("request expiry job: skipped, another instance holds the lock")
		return 0, nil
	}
	defer release()

	// The sweep gets its own deadline rather than running under the loop's
	// context, so a hung statement cannot hold the lock indefinitely.
	sweepCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	cutoff := r.now().Add(-r.cfg.MaxAge)
	expired, err := r.store.ExpireStaleOrderRequests(sweepCtx, cutoff)
	if err != nil {
		return 0, err
	}
	if len(expired) == 0 {
		r.logger.Debug("request expiry job: nothing to expire", "cutoff", cutoff.UTC().Format(time.RFC3339))
		return 0, nil
	}

	byWorkspace := make(map[uuid.UUID]int, 2)
	for _, req := range expired {
		byWorkspace[req.WorkspaceID]++
	}

	r.logger.Info("expired stale order requests",
		"count", len(expired),
		"workspaces", len(byWorkspace),
		"maxAge", r.cfg.MaxAge.String(),
		"cutoff", cutoff.UTC().Format(time.RFC3339),
	)
	// Per workspace too: the sweep is deliberately global, so the tenant each
	// row belonged to is the one thing a reader of the log cannot infer.
	for workspaceID, count := range byWorkspace {
		r.logger.Info("expired stale order requests for workspace",
			"workspaceId", workspaceID.String(),
			"count", count,
		)
	}

	return len(expired), nil
}
