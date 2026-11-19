package jobs_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/jobs"
)

func databaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("set DATABASE_URL or TEST_DATABASE_URL to run database tests")
	}
	return url
}

// withTx gives each test a transaction that is always rolled back, so the
// job runs against real PostgreSQL with real constraints while leaving the
// database exactly as it was found. It matters here more than usual: the
// sweep is deliberately global, so an uncommitted transaction is what keeps
// it from touching rows the developer cares about.
func withTx(t *testing.T) (context.Context, *sqlc.Queries, pgx.Tx) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	conn, err := pgx.Connect(ctx, databaseURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	return ctx, sqlc.New(tx), tx
}

func seedWorkspace(ctx context.Context, t *testing.T, q *sqlc.Queries, label string) sqlc.Workspace {
	t.Helper()
	ws, err := q.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{
		Name: "Expiry " + label,
		Slug: fmt.Sprintf("expiry-%s-%s", label, uuid.NewString()[:8]),
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return ws
}

// seedRequest creates a request, then backdates it and sets its status
// directly. Age and status are what the job reads, and both have to be
// arranged without going through the API.
func seedRequest(ctx context.Context, t *testing.T, q *sqlc.Queries, tx pgx.Tx, ws sqlc.Workspace, status string, age time.Duration) sqlc.OrderRequest {
	t.Helper()

	req, err := q.CreateOrderRequest(ctx, sqlc.CreateOrderRequestParams{
		WorkspaceID: ws.ID,
		BuyerName:   "Ada Lovelace",
		BuyerEmail:  "ada@example.com",
	})
	if err != nil {
		t.Fatalf("CreateOrderRequest: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE order_requests SET status = $2, created_at = now() - $3::interval WHERE id = $1`,
		req.ID, status, fmt.Sprintf("%d seconds", int64(age.Seconds())),
	); err != nil {
		t.Fatalf("backdate request: %v", err)
	}
	req.Status = status
	return req
}

func statusOf(ctx context.Context, t *testing.T, tx pgx.Tx, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM order_requests WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	return status
}

// Rules 1 to 3: only 'new' expires, and only once it is past the window.
// Accepted and rejected requests are never touched, however old they are.
func TestSweepExpiresOnlyStaleNewRequests(t *testing.T) {
	ctx, q, tx := withTx(t)
	ws := seedWorkspace(ctx, t, q, "rules")

	stale := seedRequest(ctx, t, q, tx, ws, "new", 8*24*time.Hour)
	fresh := seedRequest(ctx, t, q, tx, ws, "new", 6*24*time.Hour)
	accepted := seedRequest(ctx, t, q, tx, ws, "accepted", 30*24*time.Hour)
	rejected := seedRequest(ctx, t, q, tx, ws, "rejected", 30*24*time.Hour)

	runner := jobs.NewExpiryRunner(q, jobs.NoLocker{}, discardLogger(), jobs.ExpiryConfig{
		MaxAge: 7 * 24 * time.Hour,
	})

	count, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("expired %d requests, want 1", count)
	}

	if got := statusOf(ctx, t, tx, stale.ID); got != "rejected" {
		t.Fatalf("the stale request is %q, want rejected", got)
	}
	// Six days old is inside the window, so it keeps its chance.
	if got := statusOf(ctx, t, tx, fresh.ID); got != "new" {
		t.Fatalf("a six-day-old request became %q, want new", got)
	}
	// An accepted request has an order and reserved stock behind it. Expiring
	// one would orphan both.
	if got := statusOf(ctx, t, tx, accepted.ID); got != "accepted" {
		t.Fatalf("an accepted request became %q", got)
	}
	if got := statusOf(ctx, t, tx, rejected.ID); got != "rejected" {
		t.Fatalf("a rejected request became %q", got)
	}
}

// Rule 5: the sweep is global, and every row it returns carries the
// workspace it belonged to, so one tenant's expiry is never attributed to
// another and the per-workspace log line is accurate.
func TestSweepPreservesWorkspaceScope(t *testing.T) {
	ctx, q, tx := withTx(t)
	first := seedWorkspace(ctx, t, q, "first")
	second := seedWorkspace(ctx, t, q, "second")

	a := seedRequest(ctx, t, q, tx, first, "new", 10*24*time.Hour)
	b := seedRequest(ctx, t, q, tx, second, "new", 10*24*time.Hour)
	keep := seedRequest(ctx, t, q, tx, second, "accepted", 10*24*time.Hour)

	expired, err := q.ExpireStaleOrderRequests(ctx, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("ExpireStaleOrderRequests: %v", err)
	}

	owners := make(map[uuid.UUID]uuid.UUID, len(expired))
	for _, req := range expired {
		owners[req.ID] = req.WorkspaceID
	}
	if owners[a.ID] != first.ID {
		t.Fatalf("request from workspace %s came back under %s", first.ID, owners[a.ID])
	}
	if owners[b.ID] != second.ID {
		t.Fatalf("request from workspace %s came back under %s", second.ID, owners[b.ID])
	}
	if _, ok := owners[keep.ID]; ok {
		t.Fatal("an accepted request was expired")
	}
	if got := statusOf(ctx, t, tx, keep.ID); got != "accepted" {
		t.Fatalf("the accepted request is %q", got)
	}
}

// A second sweep over the same rows expires nothing more: once a request
// leaves 'new' it is out of scope, so the job is safe to run as often as it
// likes.
func TestSweepIsIdempotent(t *testing.T) {
	ctx, q, tx := withTx(t)
	ws := seedWorkspace(ctx, t, q, "idempotent")
	seedRequest(ctx, t, q, tx, ws, "new", 9*24*time.Hour)

	runner := jobs.NewExpiryRunner(q, jobs.NoLocker{}, discardLogger(), jobs.ExpiryConfig{
		MaxAge: 7 * 24 * time.Hour,
	})

	first, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	second, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if first != 1 || second != 0 {
		t.Fatalf("sweeps expired %d then %d, want 1 then 0", first, second)
	}
}

// Rule 7, across processes: the advisory lock is what stops two API
// instances sweeping at once. Two separate sessions, one lock.
func TestAdvisoryLockExcludesASecondHolder(t *testing.T) {
	url := databaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	// Two connections, so the second attempt is a genuinely different
	// session rather than one queueing behind the other.
	cfg.MaxConns = 4

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// A key of its own, so a running API process does not interfere.
	key := int64(uuid.New().ID())
	locker := jobs.NewAdvisoryLocker(pool, key, discardLogger())

	release, ok, err := locker.TryLock(ctx)
	if err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	if !ok {
		t.Fatal("the first TryLock did not acquire a free lock")
	}

	if _, ok, err := locker.TryLock(ctx); err != nil {
		t.Fatalf("second TryLock: %v", err)
	} else if ok {
		t.Fatal("a second session acquired a held lock")
	}

	// Released, the lock is available again.
	release()

	again, ok, err := locker.TryLock(ctx)
	if err != nil {
		t.Fatalf("third TryLock: %v", err)
	}
	if !ok {
		t.Fatal("the lock was not available after release")
	}
	again()
}

// A runner whose lock is already held does nothing, end to end.
func TestRunnerSkipsWhileAnotherSessionHoldsTheLock(t *testing.T) {
	url := databaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	key := int64(uuid.New().ID())
	locker := jobs.NewAdvisoryLocker(pool, key, discardLogger())

	release, ok, err := locker.TryLock(ctx)
	if err != nil || !ok {
		t.Fatalf("holding the lock: ok=%v err=%v", ok, err)
	}
	defer release()

	store := &stubStore{}
	runner := jobs.NewExpiryRunner(store, jobs.NewAdvisoryLocker(pool, key, discardLogger()),
		discardLogger(), jobs.ExpiryConfig{MaxAge: 7 * 24 * time.Hour})

	count, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if count != 0 || store.callCount() != 0 {
		t.Fatalf("count = %d, query ran %d times; want 0 and 0", count, store.callCount())
	}
}
