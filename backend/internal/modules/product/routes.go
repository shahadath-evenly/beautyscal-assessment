package product

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter returns the product routes. It is mounted under /api, which
// already requires a valid workspace, so these handlers can rely on one being
// present in the context.
func NewRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Get("/{productId}", h.Get)
	return r
}

// chiURLParam reads a URL parameter. Wrapped so the handler file does not
// depend on chi directly, keeping the router the only place that does.
func chiURLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
