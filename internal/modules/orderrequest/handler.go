package orderrequest

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	appmw "github.com/real-musafir/b-scale/backend/internal/middleware"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
	"github.com/real-musafir/b-scale/backend/internal/platform/validate"
)

// Handler turns HTTP requests into service calls.
type Handler struct {
	service   *Service
	validator *validate.Validator
	logger    *slog.Logger
}

// NewHandler returns a Handler.
func NewHandler(service *Service, validator *validate.Validator, logger *slog.Logger) *Handler {
	return &Handler{service: service, validator: validator, logger: logger}
}

// Create handles POST /api/order-requests.
//
// The payload carries only buyer details, product IDs and quantities. Prices
// and currencies are read from the database, so a client cannot name its own
// price, and a workspace ID in the body is ignored in favour of the header.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input CreateInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		var decodeErr *httpx.DecodeError
		if errors.As(err, &decodeErr) {
			decodeErr.WriteTo(w)
			return
		}
		httpx.BadRequest(w, "The request body could not be decoded.")
		return
	}

	fieldErrs, ok := h.validator.Struct(input)
	if !ok {
		h.logger.ErrorContext(r.Context(), "validator failed on order request input")
		httpx.Internal(w)
		return
	}
	if len(fieldErrs) > 0 {
		httpx.ValidationFailed(w, fieldErrs)
		return
	}

	workspaceID := appmw.MustWorkspaceID(r.Context())

	detail, err := h.service.Create(r.Context(), workspaceID, input)
	if err != nil {
		h.writeCreateError(w, r, workspaceID, err)
		return
	}

	httpx.Data(w, http.StatusCreated, detail)
}

// writeCreateError maps a creation failure to a response. Rule violations the
// client can fix are 422 with details; anything else is a 500 with the cause
// logged rather than returned.
func (h *Handler) writeCreateError(w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, err error) {
	var unknown *UnknownProductsError
	if errors.As(err, &unknown) {
		httpx.ValidationFailed(w, unknown.Details())
		return
	}

	var mixed *MixedCurrencyError
	if errors.As(err, &mixed) {
		httpx.ValidationFailed(w, mixed.Details())
		return
	}

	h.logger.ErrorContext(r.Context(), "create order request failed",
		"error", err, "workspaceId", workspaceID)
	httpx.Internal(w)
}

// Accept handles POST /api/order-requests/{requestId}/accept.
//
// Safe under concurrent clicks: the work happens in one transaction that
// locks the request row, so a second attempt either waits and then conflicts,
// or is rejected by the unique constraint on the order.
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	requestID, ok := h.requestID(w, r)
	if !ok {
		return
	}
	workspaceID := appmw.MustWorkspaceID(r.Context())

	accepted, err := h.service.Accept(r.Context(), workspaceID, requestID)
	if err != nil {
		h.writeTransitionError(w, r, workspaceID, requestID, "accept", err)
		return
	}

	httpx.Data(w, http.StatusOK, accepted)
}

// Reject handles POST /api/order-requests/{requestId}/reject.
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	requestID, ok := h.requestID(w, r)
	if !ok {
		return
	}
	workspaceID := appmw.MustWorkspaceID(r.Context())

	rejected, err := h.service.Reject(r.Context(), workspaceID, requestID)
	if err != nil {
		h.writeTransitionError(w, r, workspaceID, requestID, "reject", err)
		return
	}

	httpx.Data(w, http.StatusOK, rejected)
}

// requestID parses the path parameter, answering 400 when it is malformed.
func (h *Handler) requestID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(urlParam(r, "requestId"))
	if err != nil {
		httpx.BadRequest(w, "The order request ID must be a valid UUID.")
		return uuid.Nil, false
	}
	return id, true
}

// writeTransitionError maps an accept or reject failure to a response.
func (h *Handler) writeTransitionError(w http.ResponseWriter, r *http.Request, workspaceID, requestID uuid.UUID, action string, err error) {
	if errors.Is(err, ErrNotFound) {
		// Also the answer for another workspace's request.
		httpx.NotFound(w, "The requested order request was not found.")
		return
	}

	var conflict *StatusConflictError
	if errors.As(err, &conflict) {
		httpx.Conflict(w, conflict.Message())
		return
	}

	var short *InsufficientStockError
	if errors.As(err, &short) {
		// 409: the request is valid, but the catalogue cannot supply it right
		// now. No stock was taken, because the transaction rolled back.
		httpx.Error(w, http.StatusConflict, "insufficient_stock",
			"There is not enough stock to accept this request.", short.Products)
		return
	}

	h.logger.ErrorContext(r.Context(), action+" order request failed",
		"error", err, "workspaceId", workspaceID, "requestId", requestID)
	httpx.Internal(w)
}

// List handles GET /api/order-requests.
//
//	page    defaults to 1
//	limit   defaults to 10, maximum 100
//	search  matches buyer name or email, in SQL
//	status  one of new, accepted, rejected
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params, fieldErr := httpx.ParsePageParams(r)
	if fieldErr != nil {
		fieldErr.WriteTo(w)
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !IsValidStatus(status) {
		(&httpx.FieldError{
			Field:   "status",
			Message: "must be one of " + strings.Join(ValidStatuses, ", "),
		}).WriteTo(w)
		return
	}

	workspaceID := appmw.MustWorkspaceID(r.Context())

	result, err := h.service.List(r.Context(), workspaceID, params, status)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list order requests failed",
			"error", err, "workspaceId", workspaceID)
		httpx.Internal(w)
		return
	}

	httpx.List(w, http.StatusOK, result.Requests, result.Meta)
}

// Get handles GET /api/order-requests/{requestId}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	requestID, ok := h.requestID(w, r)
	if !ok {
		return
	}

	workspaceID := appmw.MustWorkspaceID(r.Context())

	detail, err := h.service.Get(r.Context(), workspaceID, requestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.NotFound(w, "The requested order request was not found.")
			return
		}
		h.logger.ErrorContext(r.Context(), "get order request failed",
			"error", err, "workspaceId", workspaceID, "requestId", requestID)
		httpx.Internal(w)
		return
	}

	httpx.Data(w, http.StatusOK, detail)
}
