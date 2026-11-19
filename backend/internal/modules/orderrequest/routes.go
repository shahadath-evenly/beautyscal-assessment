package orderrequest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter returns the order request routes. It is mounted under /api, which
// already requires a valid workspace.
//
// Accept and reject are POSTs rather than PATCHes: each is a named action
// with its own rules and failure modes, not a field update.
func NewRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Get("/{requestId}", h.Get)
	r.Post("/{requestId}/accept", h.Accept)
	r.Post("/{requestId}/reject", h.Reject)
	return r
}

// urlParam reads a URL parameter, keeping chi out of the handler file.
func urlParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
