package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// Options controls how existing rows are treated on a repeated run.
type Options struct {
	// Refresh resets price, currency and stock of products that already
	// exist back to the values in Demo. Off by default: a plain re-run must
	// not silently restore stock that a demo acceptance has decremented.
	Refresh bool
}

// WorkspaceResult reports what the seed did for one workspace.
type WorkspaceResult struct {
	ID       uuid.UUID
	Name     string
	Slug     string
	Created  int // products inserted by this run
	Updated  int // existing products refreshed by this run
	Existing int // existing products left untouched
	Total    int // products in the workspace afterwards
}

// Run loads the demo data using q, which is expected to be bound to a
// transaction so the whole seed commits or rolls back as one.
//
// It is idempotent: workspaces are matched by slug and products by
// (workspace, name), so running it repeatedly never duplicates a row.
func Run(ctx context.Context, q *sqlc.Queries, opts Options) ([]WorkspaceResult, error) {
	results := make([]WorkspaceResult, 0, len(Demo))

	for _, ws := range Demo {
		// Upserts on the unique slug, so the workspace keeps its ID across
		// runs and anything referencing it stays valid.
		workspace, err := q.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{
			Name: ws.Name,
			Slug: ws.Slug,
		})
		if err != nil {
			return nil, fmt.Errorf("seed: upsert workspace %q: %w", ws.Slug, err)
		}

		result := WorkspaceResult{
			ID:   workspace.ID,
			Name: workspace.Name,
			Slug: workspace.Slug,
		}

		for _, p := range ws.Products {
			price, err := parsePrice(p.Price)
			if err != nil {
				return nil, fmt.Errorf("seed: workspace %q, product %q: %w", ws.Slug, p.Name, err)
			}

			existing, err := q.GetProductByName(ctx, sqlc.GetProductByNameParams{
				WorkspaceID: workspace.ID,
				Name:        p.Name,
			})
			switch {
			case err == nil:
				// Already present. Only touch it when explicitly refreshing.
				if !opts.Refresh {
					result.Existing++
					continue
				}
				if _, err := q.UpdateProductCatalogue(ctx, sqlc.UpdateProductCatalogueParams{
					WorkspaceID: workspace.ID,
					ID:          existing.ID,
					Price:       price,
					Currency:    p.Currency,
					Stock:       p.Stock,
				}); err != nil {
					return nil, fmt.Errorf("seed: refresh product %q in %q: %w", p.Name, ws.Slug, err)
				}
				result.Updated++

			case errors.Is(err, pgx.ErrNoRows):
				if _, err := q.CreateProduct(ctx, sqlc.CreateProductParams{
					WorkspaceID: workspace.ID,
					Name:        p.Name,
					Price:       price,
					Currency:    p.Currency,
					Stock:       p.Stock,
				}); err != nil {
					return nil, fmt.Errorf("seed: create product %q in %q: %w", p.Name, ws.Slug, err)
				}
				result.Created++

			default:
				return nil, fmt.Errorf("seed: look up product %q in %q: %w", p.Name, ws.Slug, err)
			}
		}

		total, err := q.CountProducts(ctx, sqlc.CountProductsParams{WorkspaceID: workspace.ID})
		if err != nil {
			return nil, fmt.Errorf("seed: count products in %q: %w", ws.Slug, err)
		}
		result.Total = int(total)

		results = append(results, result)
	}

	return results, nil
}

// parsePrice converts a decimal string into the exact NUMERIC the column
// stores. A malformed price is a programming error in the demo data, so it is
// reported rather than rounded or silently zeroed.
func parsePrice(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("price %q is not a valid decimal: %w", s, err)
	}
	return n, nil
}
