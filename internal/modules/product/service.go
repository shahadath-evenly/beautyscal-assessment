package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// ErrNotFound is returned when no product matches within the workspace. A
// product in another workspace is indistinguishable from one that does not
// exist, which is deliberate: anything else would leak other tenants.
var ErrNotFound = errors.New("product: not found")

// Store is the persistence the service needs. Declared here, where it is
// consumed, so the service can be tested without a database.
type Store interface {
	List(ctx context.Context, workspaceID uuid.UUID, params httpx.PageParams) ([]sqlc.Product, int64, error)
	Get(ctx context.Context, workspaceID, productID uuid.UUID) (sqlc.Product, error)
}

// Service holds the product rules.
type Service struct {
	store Store
}

// NewService returns a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// ListResult is a page of products with its pagination metadata.
type ListResult struct {
	Products []Response
	Meta     httpx.Meta
}

// List returns one page of the workspace's catalogue.
func (s *Service) List(ctx context.Context, workspaceID uuid.UUID, params httpx.PageParams) (ListResult, error) {
	products, total, err := s.store.List(ctx, workspaceID, params)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{
		Products: toResponses(products),
		Meta:     httpx.NewMeta(params, total),
	}, nil
}

// Get returns one product in the workspace, or ErrNotFound.
func (s *Service) Get(ctx context.Context, workspaceID, productID uuid.UUID) (Response, error) {
	p, err := s.store.Get(ctx, workspaceID, productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Response{}, fmt.Errorf("%w: %s", ErrNotFound, productID)
		}
		return Response{}, err
	}
	return toResponse(p), nil
}
