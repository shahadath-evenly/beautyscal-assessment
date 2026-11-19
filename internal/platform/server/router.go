// Package server wires the HTTP router and server. Dependencies are passed in
// explicitly through Deps; nothing here reaches for a global.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	appmw "github.com/real-musafir/b-scale/backend/internal/middleware"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
	"github.com/real-musafir/b-scale/backend/internal/platform/validate"
)

// Deps are the collaborators the router needs. Feature routers (products,
// order requests) are added to this struct as they are implemented.
type Deps struct {
	Logger             *slog.Logger
	Pool               *pgxpool.Pool
	Validator          *validate.Validator
	CORSAllowedOrigins []string
	RequestTimeout     time.Duration
}

// NewRouter builds the application's HTTP handler.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(appmw.CORS(deps.CORSAllowedOrigins, 5*time.Minute))
	r.Use(chimw.Timeout(deps.RequestTimeout))

	// Liveness and readiness. /health is a static liveness signal; /ready
	// verifies the database round-trip so orchestration can wait on it.
	r.Get("/health", handleHealth())
	r.Get("/ready", handleReady(deps))

	r.Route("/api", func(api chi.Router) {
		// Feature routes are mounted here in later phases:
		//   api.Mount("/products", productRouter)
		//   api.Mount("/order-requests", orderRequestRouter)
		api.NotFound(handleNotFound)
	})

	r.NotFound(handleNotFound)
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "This method is not supported for this resource.", nil)
	})

	return r
}

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.Data(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleReady(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Pool == nil {
			httpx.Error(w, http.StatusServiceUnavailable, "not_ready", "The database is not configured.", nil)
			return
		}
		if err := deps.Pool.Ping(r.Context()); err != nil {
			// The underlying error stays in the logs, never in the response.
			deps.Logger.ErrorContext(r.Context(), "readiness ping failed", "error", err)
			httpx.Error(w, http.StatusServiceUnavailable, "not_ready", "The database is not reachable.", nil)
			return
		}
		httpx.Data(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func handleNotFound(w http.ResponseWriter, _ *http.Request) {
	httpx.Error(w, http.StatusNotFound, "not_found", "The requested resource was not found.", nil)
}
