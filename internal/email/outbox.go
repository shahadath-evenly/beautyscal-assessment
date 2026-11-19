package email

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
)

// Outbox records notifications inside the caller's transaction.
//
// It takes the *sqlc.Queries of that transaction rather than opening its own,
// which is the whole point of the pattern: the order, the stock deduction and
// the promise to email the buyer either all commit or none of them do. There
// is no window in which an order exists and nobody will ever be told.
type Outbox struct {
	maxAttempts int32
}

// NewOutbox returns an Outbox. maxAttempts bounds how many times the worker
// may try one event before giving up; zero applies a sane default.
func NewOutbox(maxAttempts int32) *Outbox {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return &Outbox{maxAttempts: maxAttempts}
}

// NotifyAccepted queues the buyer's acceptance email.
//
// It is called from inside the acceptance transaction, so it must do only
// cheap, local work: render the message and insert one row. No network call
// happens here, and none may ever be added — an HTTP call inside this
// transaction would hold the row locks for as long as the provider took to
// answer.
func (o *Outbox) NotifyAccepted(
	ctx context.Context,
	q *sqlc.Queries,
	workspaceID uuid.UUID,
	request sqlc.OrderRequest,
	items []sqlc.OrderRequestItem,
) error {
	if len(items) == 0 {
		return fmt.Errorf("email: cannot notify for request %s with no items", request.ID)
	}

	// The brand's name makes the message mean something to the buyer. It is
	// read through the same transaction, so it is the name that was current
	// when the request was accepted.
	workspace, err := q.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("email: load workspace: %w", err)
	}

	lines := make([]Line, 0, len(items))
	lineTotals := make([]string, 0, len(items))
	currency := items[0].Currency

	for _, item := range items {
		lineTotal, err := money.MultiplyByQuantity(item.UnitPrice, item.Quantity)
		if err != nil {
			return fmt.Errorf("email: line total for %s: %w", item.ProductName, err)
		}
		unitPrice, err := money.String(item.UnitPrice)
		if err != nil {
			return fmt.Errorf("email: unit price for %s: %w", item.ProductName, err)
		}
		lines = append(lines, Line{
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice:   unitPrice,
			LineTotal:   lineTotal,
		})
		lineTotals = append(lineTotals, lineTotal)
	}

	total, err := money.Sum(lineTotals...)
	if err != nil {
		return fmt.Errorf("email: total for request %s: %w", request.ID, err)
	}

	msg := RenderAccepted(AcceptedRequest{
		RequestID:  request.ID,
		BuyerName:  request.BuyerName,
		BuyerEmail: request.BuyerEmail,
		Lines:      lines,
		Total:      total,
		Currency:   currency,
		BrandName:  workspace.Name,
	})

	// The insert ignores a conflict on (workspace, request, event), so a
	// second acceptance attempt cannot queue a second message. No row coming
	// back means one was already queued, which is success, not failure.
	_, err = q.EnqueueEmail(ctx, sqlc.EnqueueEmailParams{
		WorkspaceID:    workspaceID,
		OrderRequestID: request.ID,
		EventType:      EventAccepted,
		RecipientEmail: msg.ToEmail,
		RecipientName:  msg.ToName,
		Subject:        msg.Subject,
		BodyText:       msg.Text,
		BodyHtml:       msg.HTML,
		MaxAttempts:    o.maxAttempts,
	})
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("email: enqueue acceptance for %s: %w", request.ID, err)
	}
	return nil
}

// isNoRows reports whether err is the empty result of an ON CONFLICT DO
// NOTHING insert, which means the event was already queued.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
