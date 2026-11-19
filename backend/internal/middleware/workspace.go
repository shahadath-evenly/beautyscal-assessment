package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// WorkspaceHeader carries the tenant for every API request. It is a demo
// mechanism, not authentication: a real deployment would derive the workspace
// from a verified session or token.
const WorkspaceHeader = "X-Workspace-ID"

// contextKey is unexported so no other package can write this context value.
// Only this middleware may set the workspace, which is what makes
// WorkspaceID's result trustworthy downstream.
type contextKey struct{ name string }

var workspaceIDKey = &contextKey{name: "workspace-id"}

// WorkspaceVerifier reports whether a workspace exists. It is satisfied by the
// generated sqlc Queries, and by a fake in tests.
type WorkspaceVerifier interface {
	WorkspaceExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// RequireWorkspace validates the X-Workspace-ID header, confirms the workspace
// exists, and puts its ID in the request context.
//
// Responses are deliberately specific about client mistakes and vague about
// everything else:
//   - header absent            400 workspace_header_missing
//   - header not a UUID        400 workspace_header_invalid
//   - workspace unknown        404 workspace_not_found
//   - verification failed      500, with the cause logged, not returned
//
// The header is the only accepted source of a workspace. A workspace ID in a
// request body is never trusted; handlers read the value from the context.
func RequireWorkspace(verifier WorkspaceVerifier, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(r.Header.Get(WorkspaceHeader))
			if raw == "" {
				httpx.Error(w, http.StatusBadRequest, httpx.CodeWorkspaceHeaderMissing,
					"The "+WorkspaceHeader+" header is required.", nil)
				return
			}

			workspaceID, err := uuid.Parse(raw)
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, httpx.CodeWorkspaceHeaderInvalid,
					"The "+WorkspaceHeader+" header must be a valid UUID.", nil)
				return
			}

			exists, err := verifier.WorkspaceExists(r.Context(), workspaceID)
			if err != nil {
				logger.ErrorContext(r.Context(), "workspace lookup failed",
					"error", err, "workspaceId", workspaceID)
				httpx.Internal(w)
				return
			}
			if !exists {
				// 404 rather than 403: confirming that an unknown workspace
				// exists would leak the existence of other tenants.
				httpx.Error(w, http.StatusNotFound, httpx.CodeWorkspaceNotFound,
					"The requested workspace was not found.", nil)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithWorkspaceID(r.Context(), workspaceID)))
		})
	}
}

// WithWorkspaceID returns a context carrying the workspace ID. Exported for
// tests and for background jobs that operate on a known workspace.
func WithWorkspaceID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, workspaceIDKey, id)
}

// WorkspaceID returns the workspace ID placed in the context by
// RequireWorkspace. The second result is false when the context carries none,
// which means the handler is mounted outside the middleware.
func WorkspaceID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(workspaceIDKey).(uuid.UUID)
	return id, ok
}

// MustWorkspaceID returns the workspace ID or panics.
//
// A handler behind RequireWorkspace always has one, so its absence is a wiring
// mistake rather than a runtime condition: the panic surfaces it immediately
// in development, and chi's Recoverer turns it into a 500 rather than letting
// a request proceed with a zero workspace, which would read across tenants.
func MustWorkspaceID(ctx context.Context) uuid.UUID {
	id, ok := WorkspaceID(ctx)
	if !ok {
		panic("middleware: no workspace ID in context; handler is not behind RequireWorkspace")
	}
	return id
}
