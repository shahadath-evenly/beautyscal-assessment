package email

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// Store is the slice of the generated query set the worker uses. Declaring it
// here, rather than taking *sqlc.Queries, keeps the worker testable with a
// stub and states exactly what it touches.
type Store interface {
	ClaimDueEmails(ctx context.Context, limit int32) ([]sqlc.EmailOutbox, error)
	ReleaseStaleClaims(ctx context.Context, staleAfter pgtype.Interval) ([]uuid.UUID, error)
	MarkEmailSent(ctx context.Context, arg sqlc.MarkEmailSentParams) error
	RescheduleEmail(ctx context.Context, arg sqlc.RescheduleEmailParams) error
	MarkEmailFailed(ctx context.Context, arg sqlc.MarkEmailFailedParams) error
}

// WorkerConfig tunes the polling loop.
type WorkerConfig struct {
	// PollInterval is how long the worker waits when it finds nothing. A
	// batch that filled its limit is followed immediately by another.
	PollInterval time.Duration
	// BatchSize is how many events one poll claims.
	BatchSize int32
	// Concurrency bounds the sends in flight. It is a hard bound: the worker
	// never starts an unbounded number of goroutines, however long the queue.
	Concurrency int
	// SendTimeout bounds one call to the provider.
	SendTimeout time.Duration
	// RetryBaseDelay is the first retry delay; later ones double it.
	RetryBaseDelay time.Duration
	// RetryMaxDelay caps the doubling.
	RetryMaxDelay time.Duration
	// StaleClaimAfter is how long a claimed event may sit in 'sending'
	// before another worker may take it back.
	StaleClaimAfter time.Duration
}

// withDefaults fills anything the caller left unset.
func (c WorkerConfig) withDefaults() WorkerConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = 10 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 10
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 2
	}
	if c.SendTimeout <= 0 {
		c.SendTimeout = 10 * time.Second
	}
	if c.RetryBaseDelay <= 0 {
		c.RetryBaseDelay = 30 * time.Second
	}
	if c.RetryMaxDelay <= 0 {
		c.RetryMaxDelay = 30 * time.Minute
	}
	if c.StaleClaimAfter <= 0 {
		c.StaleClaimAfter = 5 * time.Minute
	}
	return c
}

// Worker polls the outbox and delivers what it finds.
type Worker struct {
	store  Store
	sender Sender
	logger *slog.Logger
	cfg    WorkerConfig
}

// NewWorker wires a worker. Dependencies are explicit, as everywhere else:
// the store and the sender are interfaces, so tests substitute both.
func NewWorker(store Store, sender Sender, logger *slog.Logger, cfg WorkerConfig) *Worker {
	return &Worker{
		store:  store,
		sender: sender,
		logger: logger,
		cfg:    cfg.withDefaults(),
	}
}

// Run polls until ctx is cancelled.
//
// Cancellation is the shutdown signal: the current batch finishes, because
// each send carries its own timeout rather than the loop's context, and then
// Run returns. An event claimed but not completed stays in 'sending' and is
// reclaimed by ReleaseStaleClaims on the next start.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("email worker started",
		"sender", w.sender.Name(),
		"pollInterval", w.cfg.PollInterval.String(),
		"batchSize", w.cfg.BatchSize,
		"concurrency", w.cfg.Concurrency,
	)
	defer w.logger.Info("email worker stopped")

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		w.releaseStale(ctx)
		processed := w.ProcessBatch(ctx)

		// A full batch probably means more is waiting, so poll again at once
		// rather than sleeping through a backlog.
		wait := w.cfg.PollInterval
		if processed >= int(w.cfg.BatchSize) {
			wait = 0
		}
		timer.Reset(wait)
	}
}

// ProcessBatch claims one batch and delivers it. It returns how many events
// it handled, and is exported so a test can drive exactly one cycle.
func (w *Worker) ProcessBatch(ctx context.Context) int {
	events, err := w.store.ClaimDueEmails(ctx, w.cfg.BatchSize)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("email worker: claim failed", "error", err)
		}
		return 0
	}
	if len(events) == 0 {
		return 0
	}

	// A counting semaphore, not one goroutine per event: the queue may hold
	// thousands, and the provider is rate limited either way.
	slots := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup

	for _, event := range events {
		select {
		case <-ctx.Done():
			// Shutting down. Events already claimed but not started stay in
			// 'sending' and are reclaimed after StaleClaimAfter.
			wg.Wait()
			return len(events)
		case slots <- struct{}{}:
		}

		wg.Add(1)
		go func(event sqlc.EmailOutbox) {
			defer wg.Done()
			defer func() { <-slots }()
			w.deliver(ctx, event)
		}(event)
	}

	wg.Wait()
	return len(events)
}

// deliver sends one event and records the outcome.
func (w *Worker) deliver(ctx context.Context, event sqlc.EmailOutbox) {
	// The send gets its own deadline so one hung provider call cannot stall
	// shutdown or hold a worker slot indefinitely. WithoutCancel means a
	// shutdown mid-send still gets to record the result.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cfg.SendTimeout)
	defer cancel()

	messageID, err := w.sender.Send(sendCtx, Message{
		ToEmail: event.RecipientEmail,
		ToName:  event.RecipientName,
		Subject: event.Subject,
		Text:    event.BodyText,
		HTML:    event.BodyHtml,
	})

	// Recording the outcome must not inherit a cancelled context, or a
	// shutdown would leave the row stuck in 'sending'.
	recordCtx, cancelRecord := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelRecord()

	if err == nil {
		var id *string
		if messageID != "" {
			id = &messageID
		}
		if err := w.store.MarkEmailSent(recordCtx, sqlc.MarkEmailSentParams{
			ID:                event.ID,
			ProviderMessageID: id,
		}); err != nil {
			w.logger.Error("email worker: could not mark event sent",
				"eventId", event.ID, "error", err)
			return
		}
		w.logger.Info("acceptance email sent",
			"eventId", event.ID,
			"orderRequestId", event.OrderRequestID,
			"attempt", event.Attempts,
			"provider", w.sender.Name(),
		)
		return
	}

	reason := err.Error()

	// A permanent rejection, or a spent attempt budget, ends the event. The
	// row is kept with its error rather than deleted: a buyer who was never
	// told is something a person should be able to find.
	var permanentErr *PermanentError
	exhausted := event.Attempts >= event.MaxAttempts
	if errors.As(err, &permanentErr) || exhausted {
		if markErr := w.store.MarkEmailFailed(recordCtx, sqlc.MarkEmailFailedParams{
			ID:        event.ID,
			LastError: &reason,
		}); markErr != nil {
			w.logger.Error("email worker: could not mark event failed",
				"eventId", event.ID, "error", markErr)
			return
		}
		w.logger.Error("acceptance email abandoned",
			"eventId", event.ID,
			"orderRequestId", event.OrderRequestID,
			"attempts", event.Attempts,
			"permanent", errors.As(err, &permanentErr),
			"error", reason,
		)
		return
	}

	retryIn := w.backoff(event.Attempts)
	if retryErr := w.store.RescheduleEmail(recordCtx, sqlc.RescheduleEmailParams{
		ID:        event.ID,
		RetryIn:   intervalOf(retryIn),
		LastError: &reason,
	}); retryErr != nil {
		w.logger.Error("email worker: could not reschedule event",
			"eventId", event.ID, "error", retryErr)
		return
	}
	w.logger.Warn("acceptance email failed, will retry",
		"eventId", event.ID,
		"orderRequestId", event.OrderRequestID,
		"attempt", event.Attempts,
		"retryIn", retryIn.String(),
		"error", reason,
	)
}

// backoff doubles the base delay for each attempt already made, capped.
// Attempt 1 waits the base delay, attempt 2 twice it, and so on.
func (w *Worker) backoff(attempts int32) time.Duration {
	delay := w.cfg.RetryBaseDelay
	for i := int32(1); i < attempts; i++ {
		delay *= 2
		if delay >= w.cfg.RetryMaxDelay {
			return w.cfg.RetryMaxDelay
		}
	}
	if delay > w.cfg.RetryMaxDelay {
		return w.cfg.RetryMaxDelay
	}
	return delay
}

// releaseStale returns events a dead worker left claimed.
func (w *Worker) releaseStale(ctx context.Context) {
	ids, err := w.store.ReleaseStaleClaims(ctx, intervalOf(w.cfg.StaleClaimAfter))
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("email worker: could not release stale claims", "error", err)
		}
		return
	}
	if len(ids) > 0 {
		w.logger.Warn("email worker: reclaimed events left mid-send", "count", len(ids))
	}
}

// intervalOf converts a duration into the interval PostgreSQL expects.
func intervalOf(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}
