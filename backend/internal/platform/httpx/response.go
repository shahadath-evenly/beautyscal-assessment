// Package httpx holds the HTTP response envelopes and small helpers shared by
// every handler, so success and error shapes stay consistent across the API.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Envelope is the success response shape: {"data": ..., "meta": ...}.
// Meta is omitted for single-resource responses.
type Envelope struct {
	Data any   `json:"data"`
	Meta *Meta `json:"meta,omitempty"`
}

// Meta carries pagination information for list endpoints.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
}

// ErrorBody is the error response shape: {"error": {code, message, details}}.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes a single failure to the client. Details is optional and
// is used for field-level validation feedback.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent, so the response cannot be changed;
		// record it instead of surfacing internals to the client.
		slog.Error("httpx: encode response", "error", err)
	}
}

// Data writes a success envelope without pagination metadata.
func Data(w http.ResponseWriter, status int, data any) {
	JSON(w, status, Envelope{Data: data})
}

// List writes a success envelope with pagination metadata.
func List(w http.ResponseWriter, status int, data any, meta Meta) {
	JSON(w, status, Envelope{Data: data, Meta: &meta})
}

// Error writes an error envelope. Messages must stay client-safe: never pass
// SQL errors, stack traces or configuration details here.
func Error(w http.ResponseWriter, status int, code, message string, details any) {
	JSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message, Details: details}})
}
