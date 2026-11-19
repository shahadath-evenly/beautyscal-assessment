package product

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	appmw "github.com/real-musafir/b-scale/backend/internal/middleware"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// Handler turns HTTP requests into service calls. It holds no business rules:
// it parses input, delegates, and maps the result to a response.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler returns a Handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// List handles GET /api/products.
//
//	page    defaults to 1
//	limit   defaults to 10, maximum 100
//	search  matches the product name, case-insensitively, in SQL
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params, fieldErr := httpx.ParsePageParams(r)
	if fieldErr != nil {
		fieldErr.WriteTo(w)
		return
	}

	// The workspace comes from the validated header, never from the query
	// string or a body.
	workspaceID := appmw.MustWorkspaceID(r.Context())

	result, err := h.service.List(r.Context(), workspaceID, params)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list products failed",
			"error", err, "workspaceId", workspaceID)
		httpx.Internal(w)
		return
	}

	httpx.List(w, http.StatusOK, result.Products, result.Meta)
}

// Get handles GET /api/products/{productId}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(chiURLParam(r, "productId"))
	if err != nil {
		httpx.BadRequest(w, "The product ID must be a valid UUID.")
		return
	}

	workspaceID := appmw.MustWorkspaceID(r.Context())

	product, err := h.service.Get(r.Context(), workspaceID, productID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.NotFound(w, "The requested product was not found.")
			return
		}
		h.logger.ErrorContext(r.Context(), "get product failed",
			"error", err, "workspaceId", workspaceID, "productId", productID)
		httpx.Internal(w)
		return
	}

	httpx.Data(w, http.StatusOK, product)
}
