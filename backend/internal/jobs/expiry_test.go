package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/jobs"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubStore records what the runner asked for and returns what the test set.
type stubStore struct {
	mu sync.Mutex

	calls   int
	cutoffs []time.Time
	// inFlight and maxInFlight prove sweeps never overlap.
	inFlight    int
	maxInFlight int

	rows  []sqlc.OrderRequest
	err   error
	block chan struct{}
}

func (s *stubStore) ExpireStaleOrderRequests(ctx context.Context, olderThan time.Time) ([]sqlc.OrderRequest, error) {
	s.mu.Lock()
	s.calls++
	s.cutoffs = append(s.cutoffs, olderThan)
	s.inFlight++
	if s.inFlight > s.maxInFlight {
		s.maxInFlight = s.inFlight
	}
	block := s.block
	rows, err := s.rows, s.err
	s.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
		}
	}

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return rows, err
}

func (s *stubStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// stubLocker reports whether the lock was taken and released.
type stubLocker struct {
	mu        sync.Mutex
	ok        bool
	err       error
	attempts  int
	releases  int
	heldAtMax int
	held      int
}

func (l *stubLocker) TryLock(context.Context) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts++
	if l.err != nil {
		return nil, false, l.err
	}
	if !l.ok {
		return nil, false, nil
	}
	l.held++
	if l.held > l.heldAtMax {
		l.heldAtMax = l.held
	}
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.releases++
		l.held--
	}, true, nil
}

func (l *stubLocker) counts() (attempts, releases int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts, l.releases
}

func request(workspaceID uuid.UUID) sqlc.OrderRequest {
	return sqlc.OrderRequest{ID: uuid.New(), WorkspaceID: workspaceID, Status: "rejected"}
}

// The happy path: the runner reports what the query expired, and asks for a
// cutoff exactly MaxAge in the past.
func TestRunOnceExpiresAndReportsCount(t *testing.T) {
	ws := uuid.New()
	store := &stubStore{rows: []sqlc.OrderRequest{request(ws), request(ws)}}
	lock := &stubLocker{ok: true}

	runner := jobs.NewExpiryRunner(store, lock, discardLogger(), jobs.ExpiryConfig{
		MaxAge: 7 * 24 * time.Hour,
	})

	before := time.Now()
	count, err := runner.RunOnce(context.Background())
	after := time.Now()
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	if len(store.cutoffs) != 1 {
		t.Fatalf("cutoffs = %d, want 1", len(store.cutoffs))
	}
	cutoff := store.cutoffs[0]
	if cutoff.Before(before.Add(-7*24*time.Hour)) || cutoff.After(after.Add(-7*24*time.Hour)) {
		t.Fatalf("cutoff %v is not seven days before the run", cutoff)
	}
}

// MaxAge defaults to the seven days the business rule states.
func TestExpiryConfigDefaultsToSevenDays(t *testing.T) {
	store := &stubStore{}
	runner := jobs.NewExpiryRunner(store, jobs.NoLocker{}, discardLogger(), jobs.ExpiryConfig{})

	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.cutoffs) != 1 {
		t.Fatalf("cutoffs = %d, want 1", len(store.cutoffs))
	}
	age := time.Since(store.cutoffs[0])
	if age < 7*24*time.Hour-time.Minute || age > 7*24*time.Hour+time.Minute {
		t.Fatalf("default window is %v, want roughly 168h", age)
	}
}

// A lock held elsewhere means this process does no work at all. The query
// must not run: that is the whole point of the lock.
func TestRunOnceSkipsWhenTheLockIsHeld(t *testing.T) {
	store := &stubStore{rows: []sqlc.OrderRequest{request(uuid.New())}}
	lock := &stubLocker{ok: false}

	runner := jobs.NewExpiryRunner(store, lock, discardLogger(), jobs.ExpiryConfig{})

	count, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if store.callCount() != 0 {
		t.Fatalf("the query ran %d times while the lock was held", store.callCount())
	}
	// Skipping is a normal outcome, so nothing was released either.
	if _, releases := lock.counts(); releases != 0 {
		t.Fatalf("releases = %d, want 0", releases)
	}
}

// The lock is released even when the sweep fails, or a single error would
// block the job on every later run.
func TestRunOnceReleasesTheLockOnFailure(t *testing.T) {
	store := &stubStore{err: errors.New("deadlock detected")}
	lock := &stubLocker{ok: true}

	runner := jobs.NewExpiryRunner(store, lock, discardLogger(), jobs.ExpiryConfig{})

	if _, err := runner.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce returned no error")
	}
	attempts, releases := lock.counts()
	if attempts != 1 || releases != 1 {
		t.Fatalf("attempts = %d, releases = %d, want 1 and 1", attempts, releases)
	}
}

// A failure to acquire the lock is reported rather than swallowed, because it
// means the database is unreachable, not that somebody else is sweeping.
func TestRunOnceReportsLockErrors(t *testing.T) {
	store := &stubStore{}
	lock := &stubLocker{err: errors.New("connection refused")}

	runner := jobs.NewExpiryRunner(store, lock, discardLogger(), jobs.ExpiryConfig{})

	if _, err := runner.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce returned no error")
	}
	if store.callCount() != 0 {
		t.Fatalf("the query ran %d times without the lock", store.callCount())
	}
}

// Rule 7, within one process: a sweep slower than the interval must not be
// joined by the next one. The loop is sequential, so only one is ever live.
func TestRunNeverOverlapsSweeps(t *testing.T) {
	release := make(chan struct{})
	store := &stubStore{block: release}
	lock := &stubLocker{ok: true}

	runner := jobs.NewExpiryRunner(store, lock, discardLogger(), jobs.ExpiryConfig{
		// Far shorter than the sweep will take: a scheduler that fired on a
		// fixed tick would pile runs up here.
		Interval: time.Millisecond,
		MaxAge:   time.Hour,
		Timeout:  5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runner.Run(ctx)
	}()

	// Let the first sweep start and the interval elapse many times over.
	waitFor(t, func() bool { return store.callCount() >= 1 })
	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	inFlight, maxInFlight, calls := store.inFlight, store.maxInFlight, store.calls
	store.mu.Unlock()
	if inFlight != 1 {
		t.Fatalf("inFlight = %d, want 1", inFlight)
	}
	if maxInFlight != 1 {
		t.Fatalf("maxInFlight = %d: sweeps overlapped", maxInFlight)
	}
	if calls != 1 {
		t.Fatalf("calls = %d while the first sweep was still running, want 1", calls)
	}

	close(release)
	cancel()
	<-done

	// Whatever ran, every acquisition was handed back.
	attempts, releases := lock.counts()
	if attempts != releases {
		t.Fatalf("attempts = %d but releases = %d", attempts, releases)
	}
	if lock.heldAtMax > 1 {
		t.Fatalf("the lock was held %d times at once", lock.heldAtMax)
	}
}

// Cancelling the context stops the loop.
func TestRunStopsOnContextCancellation(t *testing.T) {
	store := &stubStore{}
	runner := jobs.NewExpiryRunner(store, jobs.NoLocker{}, discardLogger(), jobs.ExpiryConfig{
		Interval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runner.Run(ctx)
	}()

	waitFor(t, func() bool { return store.callCount() >= 1 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// waitFor polls cond until it holds or the test gives up.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}
