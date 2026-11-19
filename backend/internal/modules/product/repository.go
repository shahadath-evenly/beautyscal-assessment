package product

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// Querier is the slice of the generated query set this module uses. Narrowing
// it keeps the repository's dependencies visible and makes it easy to
// substitute in tests.
type Querier interface {
	ListProducts(ctx context.Context, arg sqlc.ListProductsParams) ([]sqlc.Product, error)
	CountProducts(ctx context.Context, arg sqlc.CountProductsParams) (int64, error)
	GetProduct(ctx context.Context, arg sqlc.GetProductParams) (sqlc.Product, error)
}

// Repository reads products from PostgreSQL.
type Repository struct {
	q Querier
}

// NewRepository returns a Repository backed by the generated queries.
func NewRepository(q Querier) *Repository {
	return &Repository{q: q}
}

// List returns one page of products for a workspace, together with the total
// number matching the same filter.
//
// Both the search and the paging run in PostgreSQL. The count applies the
// identical predicate, so the reported total can never disagree with the rows.
func (r *Repository) List(ctx context.Context, workspaceID uuid.UUID, params httpx.PageParams) ([]sqlc.Product, int64, error) {
	search := params.SearchOrNil()

	products, err := r.q.ListProducts(ctx, sqlc.ListProductsParams{
		WorkspaceID: workspaceID,
		Search:      search,
		PageLimit:   params.LimitValue(),
		PageOffset:  params.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("product: list: %w", err)
	}

	total, err := r.q.CountProducts(ctx, sqlc.CountProductsParams{
		WorkspaceID: workspaceID,
		Search:      search,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("product: count: %w", err)
	}

	return products, total, nil
}

// Get returns one product belonging to the workspace.
func (r *Repository) Get(ctx context.Context, workspaceID, productID uuid.UUID) (sqlc.Product, error) {
	p, err := r.q.GetProduct(ctx, sqlc.GetProductParams{WorkspaceID: workspaceID, ID: productID})
	if err != nil {
		return sqlc.Product{}, fmt.Errorf("product: get: %w", err)
	}
	return p, nil
}
