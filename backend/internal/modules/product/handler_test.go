package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	appmw "github.com/real-musafir/b-scale/backend/internal/middleware"
	"github.com/real-musafir/b-scale/backend/internal/modules/product"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
)

// fakeStore records the arguments it was called with, so the tests can assert
// that the workspace and paging reach the database layer unchanged.
type fakeStore struct {
	products []sqlc.Product
	total    int64
	err      error

	gotWorkspace uuid.UUID
	gotParams    httpx.PageParams
	calls        int
}

func (f *fakeStore) List(_ context.Context, workspaceID uuid.UUID, params httpx.PageParams) ([]sqlc.Product, int64, error) {
	f.calls++
	f.gotWorkspace = workspaceID
	f.gotParams = params
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.products, f.total, nil
}

func (f *fakeStore) Get(_ context.Context, workspaceID, productID uuid.UUID) (sqlc.Product, error) {
	f.calls++
	f.gotWorkspace = workspaceID
	if f.err != nil {
		return sqlc.Product{}, f.err
	}
	for _, p := range f.products {
		if p.ID == productID {
			return p, nil
		}
	}
	return sqlc.Product{}, pgx.ErrNoRows
}

func newProduct(t *testing.T, workspaceID uuid.UUID, name, price string, stock int32) sqlc.Product {
	t.Helper()
	n, err := money.Parse(price)
	if err != nil {
		t.Fatalf("parse price: %v", err)
	}
	return sqlc.Product{
		ID: uuid.New(), WorkspaceID: workspaceID, Name: name,
		Price: n, Currency: "EUR", Stock: stock,
	}
}

// newHandler builds the handler behind a context that already carries a
// workspace, as the middleware would supply.
func newHandler(store product.Store) (*product.Handler, uuid.UUID) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return product.NewHandler(product.NewService(store), logger), uuid.New()
}

func do(h *product.Handler, workspaceID uuid.UUID, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(appmw.WithWorkspaceID(req.Context(), workspaceID))

	res := httptest.NewRecorder()
	h.List(res, req)
	return res
}

type listBody struct {
	Data []product.Response `json:"data"`
	Meta httpx.Meta         `json:"meta"`
}

func decodeList(t *testing.T, res *httptest.ResponseRecorder) listBody {
	t.Helper()
	var body listBody
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", res.Body.String(), err)
	}
	return body
}

func TestListReturnsProductsAndMeta(t *testing.T) {
	store := &fakeStore{total: 3}
	h, ws := newHandler(store)
	store.products = []sqlc.Product{
		newProduct(t, ws, "Rose Serum", "49.90", 10),
		newProduct(t, ws, "Clay Mask", "19.5", 4),
	}

	res := do(h, ws, "/api/products")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", res.Code, res.Body.String())
	}

	body := decodeList(t, res)
	if len(body.Data) != 2 {
		t.Fatalf("returned %d products, want 2", len(body.Data))
	}

	first := body.Data[0]
	if first.Name != "Rose Serum" || first.Currency != "EUR" || first.Stock != 10 {
		t.Errorf("unexpected product %+v", first)
	}
	// Money is a fixed-point string, never a JSON number.
	if first.Price != "49.90" {
		t.Errorf("price = %q, want \"49.90\"", first.Price)
	}
	if body.Data[1].Price != "19.50" {
		t.Errorf("price = %q, want \"19.50\" with both decimals", body.Data[1].Price)
	}

	if body.Meta.Page != 1 || body.Meta.Limit != 10 {
		t.Errorf("meta = %+v, want page 1 limit 10", body.Meta)
	}
	if body.Meta.Total != 3 || body.Meta.TotalPages != 1 {
		t.Errorf("meta = %+v, want total 3, totalPages 1", body.Meta)
	}
}

func TestListUsesTheWorkspaceFromContext(t *testing.T) {
	store := &fakeStore{}
	h, ws := newHandler(store)

	// A different workspace in the query string must be ignored.
	other := uuid.New()
	res := do(h, ws, "/api/products?workspaceId="+other.String())

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if store.gotWorkspace != ws {
		t.Errorf("store received workspace %s, want %s from the header", store.gotWorkspace, ws)
	}
}

func TestListDefaultsAndPaging(t *testing.T) {
	cases := []struct {
		query      string
		wantPage   int
		wantLimit  int
		wantOffset int32
		wantSearch string
	}{
		{"", 1, 10, 0, ""},
		{"?page=3", 3, 10, 20, ""},
		{"?limit=25", 1, 25, 0, ""},
		{"?page=2&limit=5", 2, 5, 5, ""},
		{"?limit=100", 1, 100, 0, ""},
		{"?search=rose", 1, 10, 0, "rose"},
		{"?search=%20rose%20", 1, 10, 0, "rose"}, // trimmed
	}

	for _, tc := range cases {
		store := &fakeStore{}
		h, ws := newHandler(store)

		res := do(h, ws, "/api/products"+tc.query)
		if res.Code != http.StatusOK {
			t.Errorf("%q: status = %d, want 200", tc.query, res.Code)
			continue
		}

		got := store.gotParams
		if got.Page != tc.wantPage || got.Limit != tc.wantLimit {
			t.Errorf("%q: page/limit = %d/%d, want %d/%d", tc.query, got.Page, got.Limit, tc.wantPage, tc.wantLimit)
		}
		// Paging must be computed for SQL, not applied in Go.
		if got.Offset() != tc.wantOffset {
			t.Errorf("%q: offset = %d, want %d", tc.query, got.Offset(), tc.wantOffset)
		}
		if got.Search != tc.wantSearch {
			t.Errorf("%q: search = %q, want %q", tc.query, got.Search, tc.wantSearch)
		}
	}
}

func TestListRejectsInvalidPaginationParameters(t *testing.T) {
	cases := []struct {
		query string
		field string
	}{
		{"?page=0", "page"},
		{"?page=-1", "page"},
		{"?page=abc", "page"},
		{"?page=1.5", "page"},
		{"?limit=0", "limit"},
		{"?limit=-5", "limit"},
		{"?limit=abc", "limit"},
		{"?limit=101", "limit"}, // above the maximum
		{"?limit=9999", "limit"},
	}

	for _, tc := range cases {
		store := &fakeStore{}
		h, ws := newHandler(store)

		res := do(h, ws, "/api/products"+tc.query)
		if res.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", tc.query, res.Code)
			continue
		}

		var body struct {
			Error struct {
				Code    string             `json:"code"`
				Details []httpx.FieldError `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatalf("%q: decode: %v", tc.query, err)
		}
		if len(body.Error.Details) != 1 || body.Error.Details[0].Field != tc.field {
			t.Errorf("%q: details = %+v, want a %s error", tc.query, body.Error.Details, tc.field)
		}
		// A rejected request must never reach the database.
		if store.calls != 0 {
			t.Errorf("%q: store was called %d times, want 0", tc.query, store.calls)
		}
	}
}

func TestListTotalPages(t *testing.T) {
	cases := []struct {
		total int64
		limit int
		want  int
	}{
		{0, 10, 1}, // an empty list is still one page
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{25, 10, 3},
		{100, 25, 4},
	}

	for _, tc := range cases {
		store := &fakeStore{total: tc.total}
		h, ws := newHandler(store)

		res := do(h, ws, "/api/products?limit="+itoa(tc.limit))
		body := decodeList(t, res)

		if body.Meta.TotalPages != tc.want {
			t.Errorf("total %d limit %d: totalPages = %d, want %d",
				tc.total, tc.limit, body.Meta.TotalPages, tc.want)
		}
	}
}

func TestListReturnsEmptyArrayNotNull(t *testing.T) {
	store := &fakeStore{products: []sqlc.Product{}, total: 0}
	h, ws := newHandler(store)

	res := do(h, ws, "/api/products")

	// "data": null would force every client to handle a null list.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["data"]) != "[]" {
		t.Errorf("data = %s, want []", raw["data"])
	}
}

func TestListStoreFailureIsInternalAndDoesNotLeak(t *testing.T) {
	store := &fakeStore{err: errors.New(`pq: relation "products" does not exist`)}
	h, ws := newHandler(store)

	res := do(h, ws, "/api/products")

	if res.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", res.Code)
	}
	if body := res.Body.String(); contains(body, "relation") || contains(body, "pq:") {
		t.Errorf("response leaked the database error: %s", body)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
