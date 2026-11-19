package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/platform/server"
)

type fakeVerifier struct{ known uuid.UUID }

func (f fakeVerifier) WorkspaceExists(_ context.Context, id uuid.UUID) (bool, error) {
	return id == f.known, nil
}

func newTestRouter(known uuid.UUID) http.Handler {
	return server.NewRouter(server.Deps{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		Workspaces:         fakeVerifier{known: known},
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RequestTimeout:     5 * time.Second,
	})
}

// TestHealthEndpointsAreNotTenantScoped keeps orchestration working: a
// readiness probe cannot supply a workspace header.
func TestHealthEndpointsAreNotTenantScoped(t *testing.T) {
	router := newTestRouter(uuid.New())

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))

	if res.Code != http.StatusOK {
		t.Errorf("/health status = %d, want %d", res.Code, http.StatusOK)
	}

	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Data.Status)
	}
}

// TestAPIRoutesRequireAWorkspace proves the middleware is mounted on the
// whole /api tree rather than on individual routes, so a future handler
// cannot be added without tenant scoping.
func TestAPIRoutesRequireAWorkspace(t *testing.T) {
	known := uuid.New()
	router := newTestRouter(known)

	for _, path := range []string{"/api/products", "/api/order-requests", "/api/anything"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))

		if res.Code != http.StatusBadRequest {
			t.Errorf("%s without a header: status = %d, want %d", path, res.Code, http.StatusBadRequest)
		}
	}

	// With a valid workspace the request reaches routing, which has no
	// feature routes yet and so answers 404 rather than 400.
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Header.Set("X-Workspace-ID", known.String())

	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Errorf("valid workspace: status = %d, want %d (no feature routes yet)", res.Code, http.StatusNotFound)
	}
}

func TestUnknownRouteReturnsErrorEnvelope(t *testing.T) {
	router := newTestRouter(uuid.New())

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNotFound)
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", body.Error.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	router := newTestRouter(uuid.New())

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/health", nil))

	if res.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}
