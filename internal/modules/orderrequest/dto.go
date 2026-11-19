// Package orderrequest owns buyer request creation, listing and detail.
//
// Prices are never taken from the client. Every unit price and currency is
// read from the database at creation time and snapshotted onto the request's
// lines, so a later catalogue change cannot rewrite what a buyer was quoted.
package orderrequest

import (
	"time"

	"github.com/google/uuid"
)

// Status values a request can hold.
const (
	StatusNew      = "new"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// ValidStatuses lists every acceptable status filter value.
var ValidStatuses = []string{StatusNew, StatusAccepted, StatusRejected}

// CreateItemInput is one requested line. Only the product and quantity are
// accepted: a price in the payload would be ignored, so the field does not
// exist, and DecodeJSON rejects unknown fields rather than silently dropping
// one.
type CreateItemInput struct {
	ProductID uuid.UUID `json:"productId" validate:"required"`
	Quantity  int32     `json:"quantity"  validate:"required,gt=0,lte=100000"`
}

// CreateInput is the POST /api/order-requests payload.
type CreateInput struct {
	BuyerName  string            `json:"buyerName"  validate:"required,min=1,max=200"`
	BuyerEmail string            `json:"buyerEmail" validate:"required,email,max=320"`
	Items      []CreateItemInput `json:"items"      validate:"required,min=1,max=200,dive"`
}

// ItemResponse is one line of a request, with its snapshots.
type ItemResponse struct {
	ProductID   uuid.UUID `json:"productId"`
	ProductName string    `json:"productName"`
	Quantity    int32     `json:"quantity"`
	// UnitPrice and LineTotal are decimal strings, never JSON numbers.
	UnitPrice string `json:"unitPrice"`
	LineTotal string `json:"lineTotal"`
	Currency  string `json:"currency"`
}

// Response is a request without its lines, used for listings.
type Response struct {
	ID         uuid.UUID `json:"id"`
	BuyerName  string    `json:"buyerName"`
	BuyerEmail string    `json:"buyerEmail"`
	Status     string    `json:"status"`
	ItemCount  int       `json:"itemCount"`
	Total      string    `json:"total"`
	Currency   string    `json:"currency"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// DetailResponse is a request with its lines and totals.
type DetailResponse struct {
	Response
	Items []ItemResponse `json:"items"`
}

// OrderResponse is the draft order created by accepting a request.
type OrderResponse struct {
	ID        uuid.UUID `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// AcceptResponse is returned by a successful acceptance: the request in its
// new state, and the draft order the acceptance created.
type AcceptResponse struct {
	Request DetailResponse `json:"request"`
	Order   OrderResponse  `json:"order"`
}
