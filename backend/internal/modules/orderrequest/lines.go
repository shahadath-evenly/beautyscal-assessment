package orderrequest

import (
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// line is one resolved request line: the requested quantity paired with the
// product's price and currency as read from the database.
type line struct {
	ProductID   uuid.UUID
	ProductName string
	Quantity    int32
	UnitPrice   pgtype.Numeric
	Currency    string
}

// aggregateQuantities merges repeated products into one entry per product.
//
// A buyer can legitimately send the same product twice; storing it twice
// would violate the unique constraint on (workspace, request, product) and
// would make the stock arithmetic at acceptance ambiguous. Summing here keeps
// one line per product with the total quantity.
//
// The result is ordered by product ID, so creation is deterministic and
// matches the lock order the accept flow uses.
func aggregateQuantities(items []CreateItemInput) []CreateItemInput {
	totals := make(map[uuid.UUID]int32, len(items))
	for _, item := range items {
		totals[item.ProductID] += item.Quantity
	}

	out := make([]CreateItemInput, 0, len(totals))
	for id, quantity := range totals {
		out = append(out, CreateItemInput{ProductID: id, Quantity: quantity})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProductID.String() < out[j].ProductID.String()
	})
	return out
}

// productIDs returns the distinct product IDs of the aggregated items.
func productIDs(items []CreateItemInput) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ProductID)
	}
	return ids
}

// buildLines pairs each requested item with the product the database returned.
//
// It is the rule that prices come from PostgreSQL: the client supplies only a
// product and a quantity, and the unit price, currency and name are copied
// from the catalogue row. It enforces that every product exists in the
// workspace and that one request uses one currency.
//
// Pure and side-effect free, so the rules can be tested without a database,
// while the caller runs it inside the creating transaction.
func buildLines(items []CreateItemInput, products []sqlc.Product) ([]line, error) {
	byID := make(map[uuid.UUID]sqlc.Product, len(products))
	for _, p := range products {
		byID[p.ID] = p
	}

	var (
		lines   = make([]line, 0, len(items))
		missing []uuid.UUID
		seen    = map[string]bool{}
	)

	for _, item := range items {
		product, ok := byID[item.ProductID]
		if !ok {
			// The query that loaded these products was scoped to the
			// workspace, so anything absent either does not exist or belongs
			// to another tenant. Both are reported the same way.
			missing = append(missing, item.ProductID)
			continue
		}

		seen[product.Currency] = true
		lines = append(lines, line{
			ProductID:   product.ID,
			ProductName: product.Name,
			Quantity:    item.Quantity,
			UnitPrice:   product.Price,
			Currency:    product.Currency,
		})
	}

	if len(missing) > 0 {
		return nil, &UnknownProductsError{IDs: missing}
	}

	if len(seen) > 1 {
		currencies := make([]string, 0, len(seen))
		for currency := range seen {
			currencies = append(currencies, currency)
		}
		sort.Strings(currencies)
		return nil, &MixedCurrencyError{Currencies: currencies}
	}

	return lines, nil
}
