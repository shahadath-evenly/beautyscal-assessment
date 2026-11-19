package orderrequest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	appdb "github.com/real-musafir/b-scale/backend/internal/db"
	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// uniqueViolation is PostgreSQL's SQLSTATE for a unique constraint breach.
const uniqueViolation = "23505"

// Accepted is the outcome of a successful acceptance.
type Accepted struct {
	Request sqlc.OrderRequest
	Items   []sqlc.OrderRequestItem
	Order   sqlc.Order
}

// Accept reserves stock and creates the draft order for a request, in one
// transaction.
//
// The sequence, and why each step is where it is:
//
//  1. Lock the request with SELECT ... FOR UPDATE, scoped by workspace and
//     ID. Two concurrent accepts of the same request serialise here: the
//     second blocks until the first commits, then sees status 'accepted'.
//  2. Confirm the status is 'new'. Anything else is a conflict.
//  3. Aggregate the lines by product, so a product appearing more than once
//     is locked and decremented once with its total quantity.
//  4. Lock the product rows in ascending ID order. The deterministic order is
//     what keeps two transactions touching the same two products from each
//     holding the row the other wants.
//  5. Check every product has enough stock before writing anything, so an
//     oversell is reported without a partial deduction.
//  6. Decrement each product, with the stock guard repeated in the UPDATE's
//     WHERE clause.
//  7. Create exactly one draft order. The unique constraint on
//     (workspace_id, order_request_id) is the database-level backstop.
//  8. Copy the lines onto the order from the request's own snapshots, so the
//     order records the prices that were quoted.
//  9. Move the request to 'accepted', guarded on status = 'new'.
//
// Any error returns without committing, so the transaction unwinds and no
// stock change, order or status change survives. There is no in-memory lock
// anywhere: correctness comes from the database.
func (r *Repository) Accept(ctx context.Context, workspaceID, requestID uuid.UUID) (Accepted, error) {
	var result Accepted

	err := appdb.InTx(ctx, r.db, func(q *sqlc.Queries) error {
		// 1. Lock the request.
		request, err := q.GetOrderRequestForUpdate(ctx, sqlc.GetOrderRequestForUpdateParams{
			WorkspaceID: workspaceID,
			ID:          requestID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Also the answer for another workspace's request: the query
				// is scoped, so a foreign ID simply matches nothing.
				return ErrNotFound
			}
			return fmt.Errorf("orderrequest: lock request: %w", err)
		}

		// 2. Only a new request can be accepted.
		if request.Status != StatusNew {
			return &StatusConflictError{Current: request.Status, Action: "accept"}
		}

		// 3. Aggregate quantities by product, ordered by product ID.
		totals, err := q.SumOrderRequestQuantitiesByProduct(ctx, sqlc.SumOrderRequestQuantitiesByProductParams{
			WorkspaceID:    workspaceID,
			OrderRequestID: requestID,
		})
		if err != nil {
			return fmt.Errorf("orderrequest: aggregate quantities: %w", err)
		}
		if len(totals) == 0 {
			return fmt.Errorf("orderrequest: request %s has no items", requestID)
		}

		productIDs := make([]uuid.UUID, 0, len(totals))
		for _, row := range totals {
			productIDs = append(productIDs, row.ProductID)
		}

		// 4. Lock the products, in ascending ID order.
		products, err := q.LockProductsForUpdate(ctx, sqlc.LockProductsForUpdateParams{
			WorkspaceID: workspaceID,
			ProductIds:  productIDs,
		})
		if err != nil {
			return fmt.Errorf("orderrequest: lock products: %w", err)
		}

		stock := make(map[uuid.UUID]sqlc.Product, len(products))
		for _, p := range products {
			stock[p.ID] = p
		}

		// 5. Check every product before writing any of them, so a shortage
		//    on the last line cannot leave the earlier ones decremented.
		var short []ShortStock
		for _, row := range totals {
			product, ok := stock[row.ProductID]
			if !ok {
				return fmt.Errorf("orderrequest: product %s is not in workspace %s", row.ProductID, workspaceID)
			}
			if product.Stock < row.TotalQuantity {
				short = append(short, ShortStock{
					ProductID:   product.ID,
					ProductName: product.Name,
					Requested:   row.TotalQuantity,
					Available:   product.Stock,
				})
			}
		}
		if len(short) > 0 {
			return &InsufficientStockError{Products: short}
		}

		// 6. Decrement.
		for _, row := range totals {
			if _, err := q.DecrementProductStock(ctx, sqlc.DecrementProductStockParams{
				WorkspaceID: workspaceID,
				ID:          row.ProductID,
				Quantity:    row.TotalQuantity,
			}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					// The UPDATE's own stock guard matched nothing. The rows
					// are locked, so this should be unreachable; treating it
					// as a shortage keeps the failure honest either way.
					product := stock[row.ProductID]
					return &InsufficientStockError{Products: []ShortStock{{
						ProductID:   product.ID,
						ProductName: product.Name,
						Requested:   row.TotalQuantity,
						Available:   product.Stock,
					}}}
				}
				return fmt.Errorf("orderrequest: decrement stock for %s: %w", row.ProductID, err)
			}
		}

		// 7. Exactly one draft order.
		order, err := q.CreateOrder(ctx, sqlc.CreateOrderParams{
			WorkspaceID:    workspaceID,
			OrderRequestID: requestID,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
				// The second line of defence fired: an order already exists
				// for this request. Reported as a conflict, and the rollback
				// undoes the stock this attempt just took.
				return &StatusConflictError{Current: StatusAccepted, Action: "accept"}
			}
			return fmt.Errorf("orderrequest: create order: %w", err)
		}

		// 8. Order lines, built from the request's snapshots inside the
		//    database, so the quoted prices cannot drift in transit.
		if _, err := q.CopyOrderItemsFromRequest(ctx, sqlc.CopyOrderItemsFromRequestParams{
			WorkspaceID:    workspaceID,
			OrderID:        order.ID,
			OrderRequestID: requestID,
		}); err != nil {
			return fmt.Errorf("orderrequest: copy order items: %w", err)
		}

		// 9. Move the request to accepted, guarded on status = 'new'.
		accepted, err := q.MarkOrderRequestAccepted(ctx, sqlc.MarkOrderRequestAcceptedParams{
			WorkspaceID: workspaceID,
			ID:          requestID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &StatusConflictError{Current: StatusAccepted, Action: "accept"}
			}
			return fmt.Errorf("orderrequest: mark accepted: %w", err)
		}

		items, err := q.ListOrderRequestItems(ctx, sqlc.ListOrderRequestItemsParams{
			WorkspaceID:    workspaceID,
			OrderRequestID: requestID,
		})
		if err != nil {
			return fmt.Errorf("orderrequest: load items: %w", err)
		}

		result = Accepted{Request: accepted, Items: items, Order: order}
		return nil
	})
	if err != nil {
		return Accepted{}, err
	}
	return result, nil
}

// Reject marks a request rejected. It never touches stock and never creates
// an order.
//
// The request row is locked first so the decision is made against a stable
// status: without it, a reject and an accept arriving together could both
// read 'new'. The UPDATE repeats the status = 'new' guard regardless.
func (r *Repository) Reject(ctx context.Context, workspaceID, requestID uuid.UUID) (sqlc.OrderRequest, []sqlc.OrderRequestItem, error) {
	var (
		rejected sqlc.OrderRequest
		items    []sqlc.OrderRequestItem
	)

	err := appdb.InTx(ctx, r.db, func(q *sqlc.Queries) error {
		request, err := q.GetOrderRequestForUpdate(ctx, sqlc.GetOrderRequestForUpdateParams{
			WorkspaceID: workspaceID,
			ID:          requestID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("orderrequest: lock request: %w", err)
		}

		if request.Status != StatusNew {
			return &StatusConflictError{Current: request.Status, Action: "reject"}
		}

		rejected, err = q.MarkOrderRequestRejected(ctx, sqlc.MarkOrderRequestRejectedParams{
			WorkspaceID: workspaceID,
			ID:          requestID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &StatusConflictError{Current: request.Status, Action: "reject"}
			}
			return fmt.Errorf("orderrequest: mark rejected: %w", err)
		}

		items, err = q.ListOrderRequestItems(ctx, sqlc.ListOrderRequestItemsParams{
			WorkspaceID:    workspaceID,
			OrderRequestID: requestID,
		})
		if err != nil {
			return fmt.Errorf("orderrequest: load items: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlc.OrderRequest{}, nil, err
	}
	return rejected, items, nil
}
