package orderrequest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	appdb "github.com/real-musafir/b-scale/backend/internal/db"
	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// Notifier records a buyer notification inside a transaction the repository
// already holds.
//
// It is an interface declared here, where it is used, so this module does not
// depend on the email package: acceptance knows that something must be told
// about an accepted request, not how it will be delivered. The implementation
// must do only local work — rendering and one insert — because it runs with
// the request and product rows locked.
type Notifier interface {
	NotifyAccepted(
		ctx context.Context,
		q *sqlc.Queries,
		workspaceID uuid.UUID,
		request sqlc.OrderRequest,
		items []sqlc.OrderRequestItem,
	) error
}

// Repository reads and writes order requests.
type Repository struct {
	db       appdb.Beginner
	q        *sqlc.Queries
	notifier Notifier
}

// NewRepository returns a Repository. db begins the transactions creation
// needs; q serves the reads, which need none. notifier may be nil, which
// turns buyer notifications off without changing any other behaviour.
func NewRepository(db appdb.Beginner, q *sqlc.Queries, notifier Notifier) *Repository {
	return &Repository{db: db, q: q, notifier: notifier}
}

// CreateParams is what the service asks the repository to persist.
type CreateParams struct {
	BuyerName  string
	BuyerEmail string
	Items      []CreateItemInput
}

// Created is a freshly created request with its stored lines.
type Created struct {
	Request sqlc.OrderRequest
	Items   []sqlc.OrderRequestItem
}

// Create stores a request and its lines in one transaction.
//
// Inside that transaction it loads the referenced products scoped to the
// workspace, so prices and currencies come from PostgreSQL rather than from
// the client, and so a product cannot be deleted between validation and
// insert. If any step fails, nothing is written.
func (r *Repository) Create(ctx context.Context, workspaceID uuid.UUID, params CreateParams) (Created, error) {
	var created Created

	err := appdb.InTx(ctx, r.db, func(q *sqlc.Queries) error {
		products, err := q.ListProductsByIDs(ctx, sqlc.ListProductsByIDsParams{
			WorkspaceID: workspaceID,
			ProductIds:  productIDs(params.Items),
		})
		if err != nil {
			return fmt.Errorf("orderrequest: load products: %w", err)
		}

		lines, err := buildLines(params.Items, products)
		if err != nil {
			// A validation failure, surfaced as a typed error. Returning it
			// rolls the transaction back, so no request row survives.
			return err
		}

		request, err := q.CreateOrderRequest(ctx, sqlc.CreateOrderRequestParams{
			WorkspaceID: workspaceID,
			BuyerName:   params.BuyerName,
			BuyerEmail:  params.BuyerEmail,
		})
		if err != nil {
			return fmt.Errorf("orderrequest: create request: %w", err)
		}

		rows := make([]sqlc.CreateOrderRequestItemsParams, 0, len(lines))
		for _, l := range lines {
			rows = append(rows, sqlc.CreateOrderRequestItemsParams{
				WorkspaceID:    workspaceID,
				OrderRequestID: request.ID,
				ProductID:      l.ProductID,
				ProductName:    l.ProductName,
				Quantity:       l.Quantity,
				UnitPrice:      l.UnitPrice,
				Currency:       l.Currency,
			})
		}

		inserted, err := q.CreateOrderRequestItems(ctx, rows)
		if err != nil {
			return fmt.Errorf("orderrequest: create items: %w", err)
		}
		if int(inserted) != len(rows) {
			return fmt.Errorf("orderrequest: inserted %d of %d items", inserted, len(rows))
		}

		items, err := q.ListOrderRequestItems(ctx, sqlc.ListOrderRequestItemsParams{
			WorkspaceID:    workspaceID,
			OrderRequestID: request.ID,
		})
		if err != nil {
			return fmt.Errorf("orderrequest: load items: %w", err)
		}

		created = Created{Request: request, Items: items}
		return nil
	})
	if err != nil {
		return Created{}, err
	}
	return created, nil
}

// ListFilter narrows a listing. A nil Status means every status.
type ListFilter struct {
	Page   httpx.PageParams
	Status *string
}

// Listed is one page of requests with the lines belonging to them.
type Listed struct {
	Requests []sqlc.OrderRequest
	// ItemsByRequest holds the lines of every request on the page, loaded in
	// a single query so totals cost one round trip rather than one per row.
	ItemsByRequest map[uuid.UUID][]sqlc.OrderRequestItem
	Total          int64
}

// List returns one page of the workspace's requests.
func (r *Repository) List(ctx context.Context, workspaceID uuid.UUID, filter ListFilter) (Listed, error) {
	search := filter.Page.SearchOrNil()

	requests, err := r.q.ListOrderRequests(ctx, sqlc.ListOrderRequestsParams{
		WorkspaceID: workspaceID,
		Status:      filter.Status,
		Search:      search,
		PageLimit:   filter.Page.LimitValue(),
		PageOffset:  filter.Page.Offset(),
	})
	if err != nil {
		return Listed{}, fmt.Errorf("orderrequest: list: %w", err)
	}

	total, err := r.q.CountOrderRequests(ctx, sqlc.CountOrderRequestsParams{
		WorkspaceID: workspaceID,
		Status:      filter.Status,
		Search:      search,
	})
	if err != nil {
		return Listed{}, fmt.Errorf("orderrequest: count: %w", err)
	}

	listed := Listed{
		Requests:       requests,
		ItemsByRequest: map[uuid.UUID][]sqlc.OrderRequestItem{},
		Total:          total,
	}
	if len(requests) == 0 {
		return listed, nil
	}

	ids := make([]uuid.UUID, 0, len(requests))
	for _, request := range requests {
		ids = append(ids, request.ID)
	}

	items, err := r.q.ListOrderRequestItemsForRequests(ctx, sqlc.ListOrderRequestItemsForRequestsParams{
		WorkspaceID:     workspaceID,
		OrderRequestIds: ids,
	})
	if err != nil {
		return Listed{}, fmt.Errorf("orderrequest: list items: %w", err)
	}
	for _, item := range items {
		listed.ItemsByRequest[item.OrderRequestID] = append(listed.ItemsByRequest[item.OrderRequestID], item)
	}

	return listed, nil
}

// Get returns one request and its lines, scoped to the workspace.
func (r *Repository) Get(ctx context.Context, workspaceID, requestID uuid.UUID) (sqlc.OrderRequest, []sqlc.OrderRequestItem, error) {
	request, err := r.q.GetOrderRequest(ctx, sqlc.GetOrderRequestParams{
		WorkspaceID: workspaceID,
		ID:          requestID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrderRequest{}, nil, ErrNotFound
		}
		return sqlc.OrderRequest{}, nil, fmt.Errorf("orderrequest: get: %w", err)
	}

	items, err := r.q.ListOrderRequestItems(ctx, sqlc.ListOrderRequestItemsParams{
		WorkspaceID:    workspaceID,
		OrderRequestID: requestID,
	})
	if err != nil {
		return sqlc.OrderRequest{}, nil, fmt.Errorf("orderrequest: get items: %w", err)
	}

	return request, items, nil
}
