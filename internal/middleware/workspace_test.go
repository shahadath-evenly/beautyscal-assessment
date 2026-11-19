package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/middleware"
)

// fakeVerifier stands in for the generated Queries, so the middleware's
// behaviour is tested without a database.
type fakeVerifier struct {
	known map[uuid.UUID]bool
	err   error
	calls int
}

func (f *fakeVerifier) WorkspaceExists(_ context.Context, id uuid.UUID) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	return f.known[id], nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// errorBody is the error envelope the API returns.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeError(t *testing.T, res *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope from %q: %v", res.Body.String(), err)
	}
	return body
}

// newHandler wraps a terminal handler in RequireWorkspace and reports the
// workspace ID the handler observed.
func newHandler(verifier middleware.WorkspaceVerifier) (http.Handler, *uuid.UUID) {
	var seen uuid.UUID
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.MustWorkspaceID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	return middleware.RequireWorkspace(verifier, discardLogger())(next), &seen
}

func TestMissingWorkspaceHeader(t *testing.T) {
	verifier := &fakeVerifier{}
	handler, _ := newHandler(verifier)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/products", nil))

	if res.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	if code := decodeError(t, res).Error.Code; code != "workspace_header_missing" {
		t.Errorf("code = %q, want workspace_header_missing", code)
	}
	// The database must not be consulted for a request that cannot name a
	// workspace in the first place.
	if verifier.calls != 0 {
		t.Errorf("verifier called %d times, want 0", verifier.calls)
	}
}

func TestBlankWorkspaceHeaderIsTreatedAsMissing(t *testing.T) {
	handler, _ := newHandler(&fakeVerifier{})

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Header.Set(middleware.WorkspaceHeader, "   ")

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	if code := decodeError(t, res).Error.Code; code != "workspace_header_missing" {
		t.Errorf("code = %q, want workspace_header_missing", code)
	}
}

func TestInvalidWorkspaceID(t *testing.T) {
	verifier := &fakeVerifier{}
	handler, _ := newHandler(verifier)

	for _, raw := range []string{
		"not-a-uuid",
		"12345",
		"a967f929-157f-4f4e-80d9", // truncated
		"a967f929-157f-4f4e-80d9-2047ab5b6154-xx", // trailing junk
		"'; DROP TABLE workspaces; --",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
		req.Header.Set(middleware.WorkspaceHeader, raw)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		if res.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want %d", raw, res.Code, http.StatusBadRequest)
		}
		if code := decodeError(t, res).Error.Code; code != "workspace_header_invalid" {
			t.Errorf("%q: code = %q, want workspace_header_invalid", raw, code)
		}
	}

	// A header that is not a UUID never reaches the database.
	if verifier.calls != 0 {
		t.Errorf("verifier called %d times, want 0", verifier.calls)
	}
}

func TestValidWorkspacePassesThroughWithContext(t *testing.T) {
	id := uuid.New()
	verifier := &fakeVerifier{known: map[uuid.UUID]bool{id: true}}
	handler, seen := newHandler(verifier)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Header.Set(middleware.WorkspaceHeader, id.String())

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", res.Code, http.StatusOK, res.Body.String())
	}
	if *seen != id {
		t.Errorf("handler saw workspace %s, want %s", *seen, id)
	}
	if verifier.calls != 1 {
		t.Errorf("verifier called %d times, want 1", verifier.calls)
	}
}

func TestWorkspaceHeaderIsTrimmedAndCaseInsensitive(t *testing.T) {
	id := uuid.New()
	verifier := &fakeVerifier{known: map[uuid.UUID]bool{id: true}}
	handler, seen := newHandler(verifier)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	// Surrounding whitespace and an uppercase UUID must both be accepted.
	req.Header.Set(middleware.WorkspaceHeader, "  "+strings.ToUpper(id.String())+"  ")

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if *seen != id {
		t.Errorf("handler saw workspace %s, want %s", *seen, id)
	}
}

func TestUnknownWorkspace(t *testing.T) {
	// A well-formed UUID for a workspace that does not exist.
	verifier := &fakeVerifier{known: map[uuid.UUID]bool{uuid.New(): true}}
	handler, _ := newHandler(verifier)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Header.Set(middleware.WorkspaceHeader, uuid.New().String())

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	// 404, not 403: confirming the workspace exists would leak other tenants.
	if res.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", res.Code, http.StatusNotFound)
	}
	if code := decodeError(t, res).Error.Code; code != "workspace_not_found" {
		t.Errorf("code = %q, want workspace_not_found", code)
	}
}

func TestVerifierFailureReturnsInternalErrorWithoutLeaking(t *testing.T) {
	verifier := &fakeVerifier{err: errors.New("pq: connection refused to host db-primary:5432")}
	handler, _ := newHandler(verifier)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Header.Set(middleware.WorkspaceHeader, uuid.New().String())

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	// The underlying failure belongs in the logs, never in the response.
	if body := res.Body.String(); strings.Contains(body, "db-primary") || strings.Contains(body, "pq:") {
		t.Errorf("response leaked internal details: %s", body)
	}
}

// TestBodyWorkspaceIDIsNotTrusted is the tenancy rule that matters most: the
// header decides the workspace, and a different ID in the body changes
// nothing.
func TestBodyWorkspaceIDIsNotTrusted(t *testing.T) {
	headerWorkspace := uuid.New()
	bodyWorkspace := uuid.New()

	verifier := &fakeVerifier{known: map[uuid.UUID]bool{
		headerWorkspace: true,
		bodyWorkspace:   true,
	}}
	handler, seen := newHandler(verifier)

	body := strings.NewReader(`{"workspaceId":"` + bodyWorkspace.String() + `","buyerName":"Buyer"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/order-requests", body)
	req.Header.Set(middleware.WorkspaceHeader, headerWorkspace.String())
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if *seen != headerWorkspace {
		t.Errorf("handler saw workspace %s; the body's %s must be ignored", *seen, bodyWorkspace)
	}
}

func TestWorkspaceIDHelpers(t *testing.T) {
	// Absent from a bare context.
	if _, ok := middleware.WorkspaceID(context.Background()); ok {
		t.Error("WorkspaceID reported a value for a context that has none")
	}

	id := uuid.New()
	ctx := middleware.WithWorkspaceID(context.Background(), id)

	got, ok := middleware.WorkspaceID(ctx)
	if !ok || got != id {
		t.Errorf("WorkspaceID = %s, %v; want %s, true", got, ok, id)
	}
	if middleware.MustWorkspaceID(ctx) != id {
		t.Errorf("MustWorkspaceID = %s, want %s", middleware.MustWorkspaceID(ctx), id)
	}
}

func TestMustWorkspaceIDPanicsOutsideMiddleware(t *testing.T) {
	// A handler mounted outside RequireWorkspace is a wiring mistake; failing
	// loudly is safer than proceeding with a zero workspace, which would read
	// across tenants.
	defer func() {
		if recover() == nil {
			t.Error("MustWorkspaceID did not panic without a workspace in context")
		}
	}()
	middleware.MustWorkspaceID(context.Background())
}

func TestMaxBodyBytes(t *testing.T) {
	handler := middleware.MaxBodyBytes(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	within := httptest.NewRecorder()
	handler.ServeHTTP(within, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small")))
	if within.Code != http.StatusOK {
		t.Errorf("small body: status = %d, want %d", within.Code, http.StatusOK)
	}

	over := httptest.NewRecorder()
	handler.ServeHTTP(over, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 128))))
	if over.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("large body: status = %d, want %d", over.Code, http.StatusRequestEntityTooLarge)
	}
}
