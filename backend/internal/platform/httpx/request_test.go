package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

type payload struct {
	BuyerName string `json:"buyerName"`
	Quantity  int    `json:"quantity"`
}

func decode(t *testing.T, body string, contentType string) (payload, *httpx.DecodeError) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/order-requests", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	var dst payload
	err := httpx.DecodeJSON(httptest.NewRecorder(), req, &dst)
	if err == nil {
		return dst, nil
	}

	decodeErr, ok := err.(*httpx.DecodeError)
	if !ok {
		t.Fatalf("error is %T, want *httpx.DecodeError", err)
	}
	return dst, decodeErr
}

func TestDecodeJSONAcceptsValidBody(t *testing.T) {
	got, err := decode(t, `{"buyerName":"Ada","quantity":3}`, "application/json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.BuyerName != "Ada" || got.Quantity != 3 {
		t.Errorf("decoded %+v, want {Ada 3}", got)
	}
}

func TestDecodeJSONRejectsBadBodies(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"empty", "", http.StatusBadRequest, httpx.CodeBadRequest},
		{"malformed", `{"buyerName":`, http.StatusBadRequest, httpx.CodeBadRequest},
		{"wrong type", `{"quantity":"three"}`, http.StatusBadRequest, httpx.CodeBadRequest},
		// A typo in a field name is reported rather than silently dropped.
		{"unknown field", `{"buyerNmae":"Ada"}`, http.StatusBadRequest, httpx.CodeBadRequest},
		// A second document must not slip past validation.
		{"trailing content", `{"buyerName":"Ada"}{"buyerName":"Eve"}`, http.StatusBadRequest, httpx.CodeBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decode(t, tc.body, "application/json")
			if err == nil {
				t.Fatal("expected an error")
			}
			if err.Status != tc.wantStatus {
				t.Errorf("status = %d, want %d", err.Status, tc.wantStatus)
			}
			if err.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", err.Code, tc.wantCode)
			}
		})
	}
}

func TestDecodeJSONRejectsWrongContentType(t *testing.T) {
	_, err := decode(t, `{"buyerName":"Ada"}`, "text/plain")
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Status != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", err.Status, http.StatusUnsupportedMediaType)
	}
}

func TestDecodeJSONAcceptsContentTypeWithCharset(t *testing.T) {
	if _, err := decode(t, `{"buyerName":"Ada"}`, "application/json; charset=utf-8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeJSONEnforcesBodyLimit(t *testing.T) {
	huge := `{"buyerName":"` + strings.Repeat("a", int(httpx.DefaultMaxBodyBytes)+1024) + `"}`

	_, err := decode(t, huge, "application/json")
	if err == nil {
		t.Fatal("expected an error for an oversized body")
	}
	if err.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", err.Status, http.StatusRequestEntityTooLarge)
	}
	if err.Code != httpx.CodeRequestTooLarge {
		t.Errorf("code = %q, want %q", err.Code, httpx.CodeRequestTooLarge)
	}
}

func TestErrorHelpersUseTheEnvelope(t *testing.T) {
	res := httptest.NewRecorder()
	httpx.Conflict(res, "This request has already been accepted.")

	if res.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", res.Code, http.StatusConflict)
	}
	if ct := res.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if body := res.Body.String(); !strings.Contains(body, `"code":"conflict"`) {
		t.Errorf("body = %s, want a conflict code", body)
	}
}

func TestInternalErrorRevealsNothing(t *testing.T) {
	res := httptest.NewRecorder()
	httpx.Internal(res)

	if res.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	body := res.Body.String()
	for _, leak := range []string{"sql", "pgx", "panic", "goroutine", "postgres"} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Errorf("internal error leaked %q: %s", leak, body)
		}
	}
}
