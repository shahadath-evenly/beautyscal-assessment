package orderrequest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter returns the order request routes. It is mounted under /api, which
// already requires a valid workspace.
//
// Accept and reject are added in the next phase.
func NewRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Get("/{requestId}", h.Get)
	return r
}

// urlParam reads a URL parameter, keeping chi out of the handler file.
func urlParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
