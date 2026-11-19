package email_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/email"
)

// stubStore stands in for the generated queries. It records what the worker
// did rather than talking to a database, so worker behaviour is tested
// without one; the SQL itself is covered by the outbox integration test.
type stubStore struct {
	mu sync.Mutex

	pending []sqlc.EmailOutbox

	sent        []sqlc.MarkEmailSentParams
	rescheduled []sqlc.RescheduleEmailParams
	failed      []sqlc.MarkEmailFailedParams
	released    int

	claimErr error
}

func (s *stubStore) ClaimDueEmails(_ context.Context, limit int32) ([]sqlc.EmailOutbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	n := int(limit)
	if n > len(s.pending) {
		n = len(s.pending)
	}
	batch := s.pending[:n]
	s.pending = s.pending[n:]
	// The real query counts the attempt as it claims.
	claimed := make([]sqlc.EmailOutbox, 0, len(batch))
	for _, e := range batch {
		e.Attempts++
		e.Status = "sending"
		claimed = append(claimed, e)
	}
	return claimed, nil
}

func (s *stubStore) ReleaseStaleClaims(context.Context, pgtype.Interval) ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released++
	return nil, nil
}

func (s *stubStore) MarkEmailSent(_ context.Context, arg sqlc.MarkEmailSentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, arg)
	return nil
}

func (s *stubStore) RescheduleEmail(_ context.Context, arg sqlc.RescheduleEmailParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rescheduled = append(s.rescheduled, arg)
	return nil
}

func (s *stubStore) MarkEmailFailed(_ context.Context, arg sqlc.MarkEmailFailedParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, arg)
	return nil
}

func (s *stubStore) counts() (sent, rescheduled, failed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent), len(s.rescheduled), len(s.failed)
}

// stubSender records calls and returns whatever the test asked for.
type stubSender struct {
	mu     sync.Mutex
	calls  []email.Message
	result func(email.Message) (string, error)

	inFlight    atomic.Int32
	maxInFlight atomic.Int32
	hold        chan struct{}
}

func (s *stubSender) Name() string { return "stub" }

func (s *stubSender) Send(_ context.Context, msg email.Message) (string, error) {
	n := s.inFlight.Add(1)
	for {
		max := s.maxInFlight.Load()
		if n <= max || s.maxInFlight.CompareAndSwap(max, n) {
			break
		}
	}
	if s.hold != nil {
		<-s.hold
	}
	defer s.inFlight.Add(-1)

	s.mu.Lock()
	s.calls = append(s.calls, msg)
	s.mu.Unlock()

	if s.result != nil {
		return s.result(msg)
	}
	return "msg-1", nil
}

func (s *stubSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func event(attempts, maxAttempts int32) sqlc.EmailOutbox {
	return sqlc.EmailOutbox{
		ID:             uuid.New(),
		WorkspaceID:    uuid.New(),
		OrderRequestID: uuid.New(),
		EventType:      email.EventAccepted,
		RecipientEmail: "ada@example.test",
		RecipientName:  "Ada Lovelace",
		Subject:        "Your order request has been accepted",
		BodyText:       "plain",
		BodyHtml:       "<p>rich</p>",
		Status:         "pending",
		Attempts:       attempts,
		MaxAttempts:    maxAttempts,
	}
}

func testWorker(store email.Store, sender email.Sender, cfg email.WorkerConfig) *email.Worker {
	return email.NewWorker(store, sender, quietLogger(), cfg)
}

// A delivered event is closed, with the provider's ID recorded.
func TestWorkerMarksDeliveredEventsSent(t *testing.T) {
	store := &stubStore{pending: []sqlc.EmailOutbox{event(0, 5), event(0, 5)}}
	sender := &stubSender{result: func(email.Message) (string, error) { return "brevo-42", nil }}
	worker := testWorker(store, sender, email.WorkerConfig{BatchSize: 10})

	if n := worker.ProcessBatch(context.Background()); n != 2 {
		t.Fatalf("processed %d events, want 2", n)
	}
	sent, rescheduled, failed := store.counts()
	if sent != 2 || rescheduled != 0 || failed != 0 {
		t.Fatalf("sent=%d rescheduled=%d failed=%d", sent, rescheduled, failed)
	}
	if sender.callCount() != 2 {
		t.Fatalf("sender called %d times", sender.callCount())
	}
	if store.sent[0].ProviderMessageID == nil || *store.sent[0].ProviderMessageID != "brevo-42" {
		t.Error("provider message ID was not recorded")
	}
	// The message handed to the provider is the one that was stored.
	if sender.calls[0].Subject != "Your order request has been accepted" ||
		sender.calls[0].ToEmail != "ada@example.test" {
		t.Errorf("message = %+v", sender.calls[0])
	}
}

// A transient failure is retried, not abandoned, and the stored error says
// what went wrong.
func TestWorkerReschedulesTransientFailures(t *testing.T) {
	store := &stubStore{pending: []sqlc.EmailOutbox{event(0, 5)}}
	sender := &stubSender{result: func(email.Message) (string, error) {
		return "", errors.New("brevo returned 503")
	}}
	worker := testWorker(store, sender, email.WorkerConfig{
		BatchSize:      10,
		RetryBaseDelay: time.Minute,
		RetryMaxDelay:  time.Hour,
	})

	worker.ProcessBatch(context.Background())

	sent, rescheduled, failed := store.counts()
	if sent != 0 || rescheduled != 1 || failed != 0 {
		t.Fatalf("sent=%d rescheduled=%d failed=%d", sent, rescheduled, failed)
	}
	retry := store.rescheduled[0]
	if retry.LastError == nil || *retry.LastError == "" {
		t.Error("the failure reason was not recorded")
	}
	// First attempt waits the base delay.
	if got := time.Duration(retry.RetryIn.Microseconds) * time.Microsecond; got != time.Minute {
		t.Errorf("retry delay = %v, want 1m", got)
	}
}

// Each further attempt waits longer, up to the cap.
func TestWorkerBacksOffExponentially(t *testing.T) {
	cfg := email.WorkerConfig{
		BatchSize:      10,
		RetryBaseDelay: time.Minute,
		RetryMaxDelay:  10 * time.Minute,
	}
	// attempts counted *before* the claim; the claim adds one.
	cases := []struct {
		priorAttempts int32
		want          time.Duration
	}{
		{0, 1 * time.Minute},
		{1, 2 * time.Minute},
		{2, 4 * time.Minute},
		{3, 8 * time.Minute},
		{4, 10 * time.Minute}, // capped
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("after %d attempts", tc.priorAttempts), func(t *testing.T) {
			store := &stubStore{pending: []sqlc.EmailOutbox{event(tc.priorAttempts, 50)}}
			sender := &stubSender{result: func(email.Message) (string, error) {
				return "", errors.New("temporary")
			}}
			testWorker(store, sender, cfg).ProcessBatch(context.Background())

			if len(store.rescheduled) != 1 {
				t.Fatalf("rescheduled %d events", len(store.rescheduled))
			}
			got := time.Duration(store.rescheduled[0].RetryIn.Microseconds) * time.Microsecond
			if got != tc.want {
				t.Errorf("retry delay = %v, want %v", got, tc.want)
			}
		})
	}
}

// Retries are bounded: once the budget is spent the event is failed and left
// visible rather than retried forever.
func TestWorkerFailsEventsThatExhaustTheirAttempts(t *testing.T) {
	// Claiming makes this the 3rd of 3 attempts.
	store := &stubStore{pending: []sqlc.EmailOutbox{event(2, 3)}}
	sender := &stubSender{result: func(email.Message) (string, error) {
		return "", errors.New("still failing")
	}}
	worker := testWorker(store, sender, email.WorkerConfig{BatchSize: 10, RetryBaseDelay: time.Second})

	worker.ProcessBatch(context.Background())

	sent, rescheduled, failed := store.counts()
	if sent != 0 || rescheduled != 0 || failed != 1 {
		t.Fatalf("sent=%d rescheduled=%d failed=%d", sent, rescheduled, failed)
	}
	if store.failed[0].LastError == nil || *store.failed[0].LastError == "" {
		t.Error("the final error was not recorded")
	}
}

// A rejection retrying cannot fix ends the event immediately, without
// spending the remaining attempts.
func TestWorkerFailsPermanentRejectionsWithoutRetrying(t *testing.T) {
	store := &stubStore{pending: []sqlc.EmailOutbox{event(0, 5)}}
	sender := &stubSender{result: func(email.Message) (string, error) {
		return "", &email.PermanentError{Err: errors.New("invalid recipient")}
	}}
	worker := testWorker(store, sender, email.WorkerConfig{BatchSize: 10})

	worker.ProcessBatch(context.Background())

	sent, rescheduled, failed := store.counts()
	if sent != 0 || rescheduled != 0 || failed != 1 {
		t.Fatalf("sent=%d rescheduled=%d failed=%d", sent, rescheduled, failed)
	}
}

// Concurrency is bounded: a large batch must not start a goroutine per event.
func TestWorkerBoundsConcurrency(t *testing.T) {
	const (
		events      = 24
		concurrency = 3
	)

	pending := make([]sqlc.EmailOutbox, 0, events)
	for range events {
		pending = append(pending, event(0, 5))
	}
	store := &stubStore{pending: pending}

	hold := make(chan struct{})
	sender := &stubSender{hold: hold}
	// Let every send block briefly, so overlapping work is real rather than
	// a scheduling accident.
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(hold)
	}()

	worker := testWorker(store, sender, email.WorkerConfig{
		BatchSize:   events,
		Concurrency: concurrency,
	})
	worker.ProcessBatch(context.Background())

	if got := sender.maxInFlight.Load(); got > concurrency {
		t.Fatalf("%d sends ran at once, the bound is %d", got, concurrency)
	}
	if sent, _, _ := store.counts(); sent != events {
		t.Fatalf("sent %d of %d events", sent, events)
	}
}

// Nothing due is not an error, and costs no sends.
func TestWorkerDoesNothingWhenTheQueueIsEmpty(t *testing.T) {
	store := &stubStore{}
	sender := &stubSender{}
	worker := testWorker(store, sender, email.WorkerConfig{BatchSize: 10})

	if n := worker.ProcessBatch(context.Background()); n != 0 {
		t.Fatalf("processed %d events", n)
	}
	if sender.callCount() != 0 {
		t.Fatal("the sender was called with nothing to send")
	}
}

// A claim failure is logged and survived: the next tick tries again.
func TestWorkerSurvivesAClaimFailure(t *testing.T) {
	store := &stubStore{claimErr: errors.New("connection refused")}
	worker := testWorker(store, &stubSender{}, email.WorkerConfig{BatchSize: 10})

	if n := worker.ProcessBatch(context.Background()); n != 0 {
		t.Fatalf("processed %d events", n)
	}
}

// Run polls until its context is cancelled, and returns promptly when it is.
func TestWorkerRunStopsOnCancellation(t *testing.T) {
	store := &stubStore{pending: []sqlc.EmailOutbox{event(0, 5)}}
	sender := &stubSender{}
	worker := testWorker(store, sender, email.WorkerConfig{
		BatchSize:    10,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	// Give it long enough to drain the queue, then stop it.
	deadline := time.After(2 * time.Second)
	for {
		if sent, _, _ := store.counts(); sent == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the worker did not deliver the queued event")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	if store.released == 0 {
		t.Error("Run never released stale claims")
	}
}
