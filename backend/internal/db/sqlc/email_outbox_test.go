package sqlc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// seedRequest creates the request an outbox event must hang off.
func seedRequest(ctx context.Context, t *testing.T, q *sqlc.Queries, ws sqlc.Workspace) sqlc.OrderRequest {
	t.Helper()
	req, err := q.CreateOrderRequest(ctx, sqlc.CreateOrderRequestParams{
		WorkspaceID: ws.ID,
		BuyerName:   "Ada Lovelace",
		BuyerEmail:  "ada@example.com",
	})
	if err != nil {
		t.Fatalf("CreateOrderRequest: %v", err)
	}
	return req
}

func enqueue(ctx context.Context, t *testing.T, q *sqlc.Queries, ws sqlc.Workspace, req sqlc.OrderRequest) sqlc.EmailOutbox {
	t.Helper()
	ev, err := q.EnqueueEmail(ctx, sqlc.EnqueueEmailParams{
		WorkspaceID:    ws.ID,
		OrderRequestID: req.ID,
		EventType:      "order_request_accepted",
		RecipientEmail: req.BuyerEmail,
		RecipientName:  req.BuyerName,
		Subject:        "Your order request has been accepted",
		BodyText:       "plain",
		BodyHtml:       "<p>rich</p>",
		MaxAttempts:    5,
	})
	if err != nil {
		t.Fatalf("EnqueueEmail: %v", err)
	}
	return ev
}

func interval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

// The unique constraint is what stops a buyer being emailed twice, so the
// insert must absorb the conflict rather than fail the transaction it is part
// of — that transaction is an acceptance.
func TestEnqueueEmailIsIdempotentPerRequest(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "outbox-dupe")
	req := seedRequest(ctx, t, q, ws)

	first := enqueue(ctx, t, q, ws, req)

	_, err := q.EnqueueEmail(ctx, sqlc.EnqueueEmailParams{
		WorkspaceID:    ws.ID,
		OrderRequestID: req.ID,
		EventType:      "order_request_accepted",
		RecipientEmail: req.BuyerEmail,
		RecipientName:  req.BuyerName,
		Subject:        "A second attempt",
		BodyText:       "plain",
		BodyHtml:       "<p>rich</p>",
		MaxAttempts:    5,
	})
	if err != pgx.ErrNoRows {
		t.Fatalf("second enqueue error = %v, want pgx.ErrNoRows from DO NOTHING", err)
	}

	events, err := q.ListEmailEventsForRequest(ctx, sqlc.ListEmailEventsForRequestParams{
		WorkspaceID: ws.ID, OrderRequestID: req.ID,
	})
	if err != nil {
		t.Fatalf("ListEmailEventsForRequest: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("%d events queued, want 1", len(events))
	}
	if events[0].ID != first.ID || events[0].Subject != first.Subject {
		t.Error("the second enqueue overwrote the first event")
	}
}

// Claiming moves the event out of the queue and counts the attempt, so the
// same event cannot be picked up twice by the same poll.
func TestClaimDueEmailsTakesEventsOnce(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "outbox-claim")
	req := seedRequest(ctx, t, q, ws)
	queued := enqueue(ctx, t, q, ws, req)

	claimed, err := q.ClaimDueEmails(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueEmails: %v", err)
	}
	if !containsEvent(claimed, queued.ID) {
		t.Fatal("the queued event was not claimed")
	}
	for _, ev := range claimed {
		if ev.ID != queued.ID {
			continue
		}
		if ev.Status != "sending" {
			t.Errorf("status = %q, want sending", ev.Status)
		}
		if ev.Attempts != 1 {
			t.Errorf("attempts = %d, want 1", ev.Attempts)
		}
	}

	again, err := q.ClaimDueEmails(ctx, 10)
	if err != nil {
		t.Fatalf("second ClaimDueEmails: %v", err)
	}
	if containsEvent(again, queued.ID) {
		t.Fatal("the same event was claimed twice")
	}
}

// An event scheduled for later is not due, so a retry waits out its backoff
// instead of spinning.
func TestRescheduleEmailDefersTheNextAttempt(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "outbox-retry")
	req := seedRequest(ctx, t, q, ws)
	queued := enqueue(ctx, t, q, ws, req)

	if _, err := q.ClaimDueEmails(ctx, 10); err != nil {
		t.Fatalf("ClaimDueEmails: %v", err)
	}

	reason := "brevo returned 503"
	if err := q.RescheduleEmail(ctx, sqlc.RescheduleEmailParams{
		ID:        queued.ID,
		RetryIn:   interval(10 * time.Minute),
		LastError: &reason,
	}); err != nil {
		t.Fatalf("RescheduleEmail: %v", err)
	}

	after, err := q.GetEmailEvent(ctx, queued.ID)
	if err != nil {
		t.Fatalf("GetEmailEvent: %v", err)
	}
	if after.Status != "pending" {
		t.Errorf("status = %q, want pending", after.Status)
	}
	if after.LastError == nil || *after.LastError != reason {
		t.Errorf("lastError = %v", after.LastError)
	}
	if !after.NextAttemptAt.After(time.Now().Add(5 * time.Minute)) {
		t.Errorf("nextAttemptAt = %v, want roughly ten minutes out", after.NextAttemptAt)
	}
	// Not due, so a poll must leave it alone.
	claimed, err := q.ClaimDueEmails(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueEmails: %v", err)
	}
	if containsEvent(claimed, queued.ID) {
		t.Fatal("an event scheduled for later was claimed now")
	}
}

func TestMarkEmailSentClosesTheEvent(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "outbox-sent")
	req := seedRequest(ctx, t, q, ws)
	queued := enqueue(ctx, t, q, ws, req)

	id := "<202611.1@brevo>"
	if err := q.MarkEmailSent(ctx, sqlc.MarkEmailSentParams{
		ID: queued.ID, ProviderMessageID: &id,
	}); err != nil {
		t.Fatalf("MarkEmailSent: %v", err)
	}

	after, err := q.GetEmailEvent(ctx, queued.ID)
	if err != nil {
		t.Fatalf("GetEmailEvent: %v", err)
	}
	if after.Status != "sent" {
		t.Errorf("status = %q", after.Status)
	}
	// The schema requires a sent event to say when it was sent.
	if !after.SentAt.Valid {
		t.Error("sentAt was not recorded")
	}
	if after.ProviderMessageID == nil || *after.ProviderMessageID != id {
		t.Errorf("providerMessageId = %v", after.ProviderMessageID)
	}

	claimed, err := q.ClaimDueEmails(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueEmails: %v", err)
	}
	if containsEvent(claimed, queued.ID) {
		t.Fatal("a sent event was claimed again")
	}
}

func TestMarkEmailFailedRemovesItFromTheQueue(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "outbox-failed")
	req := seedRequest(ctx, t, q, ws)
	queued := enqueue(ctx, t, q, ws, req)

	reason := "brevo rejected the message with 400"
	if err := q.MarkEmailFailed(ctx, sqlc.MarkEmailFailedParams{
		ID: queued.ID, LastError: &reason,
	}); err != nil {
		t.Fatalf("MarkEmailFailed: %v", err)
	}

	after, err := q.GetEmailEvent(ctx, queued.ID)
	if err != nil {
		t.Fatalf("GetEmailEvent: %v", err)
	}
	if after.Status != "failed" || after.LastError == nil {
		t.Fatalf("status = %q, lastError = %v", after.Status, after.LastError)
	}
	// Kept, not deleted: a buyer who was never told must stay findable.
	if after.SentAt.Valid {
		t.Error("a failed event claims to have been sent")
	}

	claimed, err := q.ClaimDueEmails(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueEmails: %v", err)
	}
	if containsEvent(claimed, queued.ID) {
		t.Fatal("a failed event was claimed again")
	}
}

// A worker that dies mid-send leaves the row in 'sending'. Without reclaim it
// would never be retried.
func TestReleaseStaleClaimsReturnsAbandonedEvents(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "outbox-stale")
	req := seedRequest(ctx, t, q, ws)
	queued := enqueue(ctx, t, q, ws, req)

	if _, err := q.ClaimDueEmails(ctx, 10); err != nil {
		t.Fatalf("ClaimDueEmails: %v", err)
	}

	// A fresh claim is not stale, so a long-running send is left alone.
	ids, err := q.ReleaseStaleClaims(ctx, interval(5*time.Minute))
	if err != nil {
		t.Fatalf("ReleaseStaleClaims: %v", err)
	}
	if containsID(ids, queued.ID) {
		t.Fatal("an event claimed moments ago was reclaimed")
	}

	// A negative threshold puts the cut-off in the future, so every claim
	// counts as abandoned. (Inside one transaction now() is frozen at its
	// start, so waiting is not an option here.)
	ids, err = q.ReleaseStaleClaims(ctx, interval(-time.Second))
	if err != nil {
		t.Fatalf("ReleaseStaleClaims: %v", err)
	}
	if !containsID(ids, queued.ID) {
		t.Fatal("the abandoned event was not reclaimed")
	}

	after, err := q.GetEmailEvent(ctx, queued.ID)
	if err != nil {
		t.Fatalf("GetEmailEvent: %v", err)
	}
	if after.Status != "pending" {
		t.Errorf("status = %q, want pending", after.Status)
	}
	// The attempt it already spent is not refunded, so a crash loop still
	// exhausts its budget.
	if after.Attempts != 1 {
		t.Errorf("attempts = %d, want the spent attempt to be kept", after.Attempts)
	}
}

func containsEvent(events []sqlc.EmailOutbox, id uuid.UUID) bool {
	for _, e := range events {
		if e.ID == id {
			return true
		}
	}
	return false
}

func containsID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}
