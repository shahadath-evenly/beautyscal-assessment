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
	Logger    *slog.Logger
	Pool      *pgxpool.Pool
	Validator *validate.Validator
	// Workspaces verifies the tenant behind X-Workspace-ID.
	Workspaces         appmw.WorkspaceVerifier
	CORSAllowedOrigins []string
	RequestTimeout     time.Duration
	// MaxBodyBytes caps request bodies; zero applies the default.
	MaxBodyBytes int64
}

// NewRouter builds the application's HTTP handler.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	maxBody := deps.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = httpx.DefaultMaxBodyBytes
	}

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(appmw.CORS(deps.CORSAllowedOrigins, 5*time.Minute))
	r.Use(appmw.MaxBodyBytes(maxBody))
	r.Use(chimw.Timeout(deps.RequestTimeout))

	// Liveness and readiness. /health is a static liveness signal; /ready
	// verifies the database round-trip so orchestration can wait on it.
	r.Get("/health", handleHealth())
	r.Get("/ready", handleReady(deps))

	r.Route("/api", func(api chi.Router) {
		// Everything under /api is tenant-scoped: the workspace is resolved
		// once here, so no handler has to parse the header itself, and none
		// can forget to.
		api.Use(appmw.RequireWorkspace(deps.Workspaces, deps.Logger))

		// Feature routes are mounted here in later phases:
		//   api.Mount("/products", productRouter)
		//   api.Mount("/order-requests", orderRequestRouter)

		// A catch-all registered as a route, not as NotFound: chi answers an
		// unmatched path from the parent mux, which would skip this group's
		// middleware and let /api/<typo> reply 404 without ever checking the
		// workspace header. Registering a route keeps every /api path inside
		// the tenant-scoped chain. More specific routes still win.
		api.Handle("/*", http.HandlerFunc(handleNotFound))
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
