package orderrequest

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
)

// Store is the persistence the service needs, declared where it is consumed.
type Store interface {
	Create(ctx context.Context, workspaceID uuid.UUID, params CreateParams) (Created, error)
	List(ctx context.Context, workspaceID uuid.UUID, filter ListFilter) (Listed, error)
	Get(ctx context.Context, workspaceID, requestID uuid.UUID) (sqlc.OrderRequest, []sqlc.OrderRequestItem, error)
}

// Service holds the order request rules.
type Service struct {
	store Store
}

// NewService returns a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates and stores a buyer's request.
//
// Repeated products are merged before anything is written, so one product
// yields one line with the summed quantity.
func (s *Service) Create(ctx context.Context, workspaceID uuid.UUID, input CreateInput) (DetailResponse, error) {
	created, err := s.store.Create(ctx, workspaceID, CreateParams{
		BuyerName:  strings.TrimSpace(input.BuyerName),
		BuyerEmail: strings.TrimSpace(input.BuyerEmail),
		Items:      aggregateQuantities(input.Items),
	})
	if err != nil {
		return DetailResponse{}, err
	}
	return toDetail(created.Request, created.Items)
}

// ListResult is a page of requests with its pagination metadata.
type ListResult struct {
	Requests []Response
	Meta     httpx.Meta
}

// List returns one page of the workspace's requests, newest first.
func (s *Service) List(ctx context.Context, workspaceID uuid.UUID, params httpx.PageParams, status string) (ListResult, error) {
	filter := ListFilter{Page: params}
	if status != "" {
		filter.Status = &status
	}

	listed, err := s.store.List(ctx, workspaceID, filter)
	if err != nil {
		return ListResult{}, err
	}

	responses := make([]Response, 0, len(listed.Requests))
	for _, request := range listed.Requests {
		summary, err := toSummary(request, listed.ItemsByRequest[request.ID])
		if err != nil {
			return ListResult{}, err
		}
		responses = append(responses, summary)
	}

	return ListResult{
		Requests: responses,
		Meta:     httpx.NewMeta(params, listed.Total),
	}, nil
}

// Get returns one request with its lines and totals, or ErrNotFound.
func (s *Service) Get(ctx context.Context, workspaceID, requestID uuid.UUID) (DetailResponse, error) {
	request, items, err := s.store.Get(ctx, workspaceID, requestID)
	if err != nil {
		return DetailResponse{}, err
	}
	return toDetail(request, items)
}

// IsValidStatus reports whether a status filter value is acceptable.
func IsValidStatus(status string) bool {
	return slices.Contains(ValidStatuses, status)
}

// toSummary renders a request without its lines, but with the totals a
// listing needs.
func toSummary(request sqlc.OrderRequest, items []sqlc.OrderRequestItem) (Response, error) {
	total, currency, err := totals(items)
	if err != nil {
		return Response{}, err
	}
	return Response{
		ID:         request.ID,
		BuyerName:  request.BuyerName,
		BuyerEmail: request.BuyerEmail,
		Status:     request.Status,
		ItemCount:  len(items),
		Total:      total,
		Currency:   currency,
		CreatedAt:  request.CreatedAt,
		UpdatedAt:  request.UpdatedAt,
	}, nil
}

// toDetail renders a request with its lines.
func toDetail(request sqlc.OrderRequest, items []sqlc.OrderRequestItem) (DetailResponse, error) {
	summary, err := toSummary(request, items)
	if err != nil {
		return DetailResponse{}, err
	}

	lines := make([]ItemResponse, 0, len(items))
	for _, item := range items {
		lineTotal, err := money.MultiplyByQuantity(item.UnitPrice, item.Quantity)
		if err != nil {
			return DetailResponse{}, fmt.Errorf("orderrequest: line total for %s: %w", item.ProductID, err)
		}
		lines = append(lines, ItemResponse{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice:   money.MustString(item.UnitPrice),
			LineTotal:   lineTotal,
			Currency:    item.Currency,
		})
	}

	return DetailResponse{Response: summary, Items: lines}, nil
}

// totals sums the lines. Creation enforces one currency per request, so a
// single total is meaningful; unlike currencies are never added.
func totals(items []sqlc.OrderRequestItem) (total string, currency string, err error) {
	if len(items) == 0 {
		return "0.00", "", nil
	}

	lineTotals := make([]string, 0, len(items))
	for _, item := range items {
		if currency == "" {
			currency = item.Currency
		} else if item.Currency != currency {
			// Unreachable through the API, which rejects mixed currencies at
			// creation. Reported rather than silently adding unlike amounts.
			return "", "", &MixedCurrencyError{Currencies: []string{currency, item.Currency}}
		}

		lineTotal, err := money.MultiplyByQuantity(item.UnitPrice, item.Quantity)
		if err != nil {
			return "", "", fmt.Errorf("orderrequest: line total for %s: %w", item.ProductID, err)
		}
		lineTotals = append(lineTotals, lineTotal)
	}

	total, err = money.Sum(lineTotals...)
	if err != nil {
		return "", "", fmt.Errorf("orderrequest: total: %w", err)
	}
	return total, currency, nil
}
