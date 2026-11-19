// Package product serves the workspace-scoped product catalogue.
//
// The layering is repository -> service -> handler, each constructed
// explicitly. Every operation takes the workspace ID as its first argument:
// it is never read from a global, a request body, or re-derived from the
// request.
package product

import (
	"time"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
)

// Response is one product as the API returns it.
type Response struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	// Price is a decimal string such as "49.90". Never a JSON number: most
	// clients parse those as doubles, which cannot represent every amount
	// exactly.
	Price     string    `json:"price"`
	Currency  string    `json:"currency"`
	Stock     int32     `json:"stock"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// toResponse maps a database row to the API shape. workspace_id is
// deliberately not exposed: the client already knows it, and echoing it
// invites treating a body value as authoritative.
func toResponse(p sqlc.Product) Response {
	return Response{
		ID:        p.ID,
		Name:      p.Name,
		Price:     money.MustString(p.Price),
		Currency:  p.Currency,
		Stock:     p.Stock,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func toResponses(products []sqlc.Product) []Response {
	out := make([]Response, 0, len(products))
	for _, p := range products {
		out = append(out, toResponse(p))
	}
	return out
}
