package orderrequest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/email"
)

// eventsFor returns the outbox rows queued for a request.
func (f *fixture) eventsFor(t *testing.T, workspaceID, requestID uuid.UUID) []sqlc.EmailOutbox {
	t.Helper()
	events, err := f.queries.ListEmailEventsForRequest(f.ctx, sqlc.ListEmailEventsForRequestParams{
		WorkspaceID:    workspaceID,
		OrderRequestID: requestID,
	})
	if err != nil {
		t.Fatalf("ListEmailEventsForRequest: %v", err)
	}
	return events
}

// submit creates a request through the API and returns its ID.
func (f *fixture) submit(t *testing.T, ws sqlc.Workspace, items map[uuid.UUID]int32) uuid.UUID {
	t.Helper()
	res := f.do(t, http.MethodPost, "/api/order-requests", ws.ID.String(), createBody(items))
	if res.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", res.Code, res.Body.String())
	}
	var body detailBody
	decodeInto(t, res, &body)
	return body.Data.ID
}

// Accepting queues exactly one event, in the same transaction, carrying the
// message the buyer will receive.
func TestAcceptQueuesTheBuyerNotification(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Absolute Serum 30ml", "68.00", "EUR")
	cream := f.createProduct(t, f.workspace, "Night Repair Cream 50ml", "89.90", "EUR")

	requestID := f.submit(t, f.workspace, map[uuid.UUID]int32{serum.ID: 2, cream.ID: 1})

	if events := f.eventsFor(t, f.workspace.ID, requestID); len(events) != 0 {
		t.Fatalf("a request queued %d events before acceptance; it should queue none", len(events))
	}

	res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if res.Code != http.StatusOK {
		t.Fatalf("accept: status %d, body %s", res.Code, res.Body.String())
	}

	events := f.eventsFor(t, f.workspace.ID, requestID)
	if len(events) != 1 {
		t.Fatalf("queued %d events, want exactly 1", len(events))
	}
	ev := events[0]

	if ev.EventType != email.EventAccepted {
		t.Errorf("eventType = %q", ev.EventType)
	}
	if ev.Status != "pending" {
		t.Errorf("status = %q, want pending", ev.Status)
	}
	if ev.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 before the worker runs", ev.Attempts)
	}
	if ev.RecipientEmail != "ada@example.com" || ev.RecipientName != "Ada Lovelace" {
		t.Errorf("recipient = %q <%q>", ev.RecipientName, ev.RecipientEmail)
	}
	if ev.WorkspaceID != f.workspace.ID {
		t.Errorf("workspaceId = %s, want %s", ev.WorkspaceID, f.workspace.ID)
	}
	if ev.SentAt.Valid {
		t.Error("sentAt is set on an event nobody has sent")
	}

	// The stored body is the whole message: products, quantities, the total
	// and its currency.
	for _, want := range []string{
		"Ada Lovelace",
		requestID.String(),
		"Rose Absolute Serum 30ml",
		"Night Repair Cream 50ml",
		"68.00",
		"136.00",
		"225.90",
		"EUR",
	} {
		if !strings.Contains(ev.BodyText, want) {
			t.Errorf("queued body is missing %q", want)
		}
	}
}

// Rejection tells the buyer nothing through this channel, and must not touch
// the queue.
func TestRejectQueuesNoNotification(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")
	requestID := f.submit(t, f.workspace, map[uuid.UUID]int32{serum.ID: 1})

	res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/reject",
		f.workspace.ID.String(), "")
	if res.Code != http.StatusOK {
		t.Fatalf("reject: status %d, body %s", res.Code, res.Body.String())
	}

	if events := f.eventsFor(t, f.workspace.ID, requestID); len(events) != 0 {
		t.Fatalf("rejection queued %d events", len(events))
	}
}

// A second acceptance is a conflict, and must not queue a second message:
// the buyer is told once.
func TestSecondAcceptQueuesNoSecondNotification(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")
	requestID := f.submit(t, f.workspace, map[uuid.UUID]int32{serum.ID: 1})

	first := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if first.Code != http.StatusOK {
		t.Fatalf("first accept: status %d", first.Code)
	}
	second := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if second.Code != http.StatusConflict {
		t.Fatalf("second accept: status %d, want 409", second.Code)
	}

	if events := f.eventsFor(t, f.workspace.ID, requestID); len(events) != 1 {
		t.Fatalf("queued %d events after two accepts, want 1", len(events))
	}
}

// An acceptance that fails leaves nothing behind — no order, no stock change
// and no promise to email anyone.
func TestFailedAcceptQueuesNoNotification(t *testing.T) {
	f := newFixture(t)
	// Stock is 100 in this fixture, so 150 cannot be satisfied.
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")
	requestID := f.submit(t, f.workspace, map[uuid.UUID]int32{serum.ID: 150})

	res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if res.Code != http.StatusConflict {
		t.Fatalf("accept: status %d, want 409; body %s", res.Code, res.Body.String())
	}

	if events := f.eventsFor(t, f.workspace.ID, requestID); len(events) != 0 {
		t.Fatalf("a rolled-back acceptance left %d queued events", len(events))
	}
}
