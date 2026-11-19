package orderrequest

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// ErrNotFound is returned when no request matches within the workspace. A
// request belonging to another workspace is indistinguishable from one that
// does not exist.
var ErrNotFound = errors.New("orderrequest: not found")

// UnknownProductsError reports product IDs that do not exist in the current
// workspace. The two cases are deliberately merged: telling a client that a
// product exists but belongs elsewhere would leak another tenant's catalogue.
type UnknownProductsError struct {
	IDs []uuid.UUID
}

func (e *UnknownProductsError) Error() string {
	ids := make([]string, 0, len(e.IDs))
	for _, id := range e.IDs {
		ids = append(ids, id.String())
	}
	return "orderrequest: unknown products: " + strings.Join(ids, ", ")
}

// Details renders the failure as field errors for the error envelope.
func (e *UnknownProductsError) Details() []httpx.FieldError {
	out := make([]httpx.FieldError, 0, len(e.IDs))
	for _, id := range e.IDs {
		out = append(out, httpx.FieldError{
			Field:   "items.productId",
			Message: fmt.Sprintf("product %s does not exist in this workspace", id),
		})
	}
	return out
}

// MixedCurrencyError reports a request whose lines span several currencies.
// One request must use one currency, or its total would be meaningless.
type MixedCurrencyError struct {
	Currencies []string
}

func (e *MixedCurrencyError) Error() string {
	return "orderrequest: mixed currencies: " + strings.Join(e.Currencies, ", ")
}

// Details renders the failure as field errors for the error envelope.
func (e *MixedCurrencyError) Details() []httpx.FieldError {
	return []httpx.FieldError{{
		Field: "items",
		Message: "all items must use the same currency, but this request mixes " +
			strings.Join(e.Currencies, " and "),
	}}
}
