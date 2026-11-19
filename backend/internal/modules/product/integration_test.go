package product_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/modules/product"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
	"github.com/real-musafir/b-scale/backend/internal/platform/server"
)

// apiFixture is the full stack over a transaction that is always rolled back:
// real router, real middleware, real SQL, no residue.
type apiFixture struct {
	router    http.Handler
	queries   *sqlc.Queries
	ctx       context.Context
	workspace sqlc.Workspace
	other     sqlc.Workspace
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("set DATABASE_URL or TEST_DATABASE_URL to run database tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	q := sqlc.New(tx)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	f := &apiFixture{
		queries: q,
		ctx:     ctx,
		router: server.NewRouter(server.Deps{
			Logger:             logger,
			Workspaces:         q,
			Products:           product.NewHandler(product.NewService(product.NewRepository(q)), logger),
			CORSAllowedOrigins: []string{"*"},
			RequestTimeout:     10 * time.Second,
		}),
	}

	f.workspace = f.createWorkspace(t, "products-main")
	f.other = f.createWorkspace(t, "products-other")
	return f
}

func (f *apiFixture) createWorkspace(t *testing.T, label string) sqlc.Workspace {
	t.Helper()
	ws, err := f.queries.CreateWorkspace(f.ctx, sqlc.CreateWorkspaceParams{
		Name: "Test " + label,
		Slug: label + "-" + uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return ws
}

func (f *apiFixture) createProduct(t *testing.T, ws sqlc.Workspace, name, price, currency string, stock int32) sqlc.Product {
	t.Helper()
	amount, err := money.Parse(price)
	if err != nil {
		t.Fatalf("parse price: %v", err)
	}
	p, err := f.queries.CreateProduct(f.ctx, sqlc.CreateProductParams{
		WorkspaceID: ws.ID, Name: name, Price: amount, Currency: currency, Stock: stock,
	})
	if err != nil {
		t.Fatalf("CreateProduct(%s): %v", name, err)
	}
	return p
}

// get issues a request as a workspace, through the real middleware chain.
func (f *apiFixture) get(t *testing.T, workspaceID, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if workspaceID != "" {
		req.Header.Set("X-Workspace-ID", workspaceID)
	}
	res := httptest.NewRecorder()
	f.router.ServeHTTP(res, req)
	return res
}

func (f *apiFixture) list(t *testing.T, workspaceID, target string) listBody {
	t.Helper()
	res := f.get(t, workspaceID, target)
	if res.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, body %s", target, res.Code, res.Body.String())
	}
	var body listBody
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestIntegrationListProducts(t *testing.T) {
	f := newAPIFixture(t)
	f.createProduct(t, f.workspace, "Rose Absolute Serum", "68.00", "EUR", 120)
	f.createProduct(t, f.workspace, "Night Repair Cream", "89.90", "EUR", 65)

	body := f.list(t, f.workspace.ID.String(), "/api/products")

	if len(body.Data) != 2 {
		t.Fatalf("returned %d products, want 2", len(body.Data))
	}
	if body.Meta.Total != 2 || body.Meta.TotalPages != 1 {
		t.Errorf("meta = %+v, want total 2, totalPages 1", body.Meta)
	}

	// Prices survive the whole round trip exactly.
	prices := map[string]string{}
	for _, p := range body.Data {
		prices[p.Name] = p.Price
	}
	if prices["Rose Absolute Serum"] != "68.00" {
		t.Errorf("price = %q, want 68.00", prices["Rose Absolute Serum"])
	}
	if prices["Night Repair Cream"] != "89.90" {
		t.Errorf("price = %q, want 89.90", prices["Night Repair Cream"])
	}
}

func TestIntegrationSearchRunsInPostgres(t *testing.T) {
	f := newAPIFixture(t)
	f.createProduct(t, f.workspace, "Rose Absolute Serum", "68.00", "EUR", 10)
	f.createProduct(t, f.workspace, "Rose Clay Mask", "24.00", "EUR", 10)
	f.createProduct(t, f.workspace, "Kelp Hair Oil", "38.00", "EUR", 10)

	// Case-insensitive, partial, and the total reflects the same filter.
	body := f.list(t, f.workspace.ID.String(), "/api/products?search=ROSE")
	if len(body.Data) != 2 {
		t.Errorf("search returned %d products, want 2", len(body.Data))
	}
	if body.Meta.Total != 2 {
		t.Errorf("meta total = %d, want 2 (the count must use the same filter)", body.Meta.Total)
	}

	// A term matching nothing yields an empty page, not an error.
	none := f.list(t, f.workspace.ID.String(), "/api/products?search=zzzznothing")
	if len(none.Data) != 0 || none.Meta.Total != 0 {
		t.Errorf("no-match search returned %d products, total %d", len(none.Data), none.Meta.Total)
	}
	if none.Meta.TotalPages != 1 {
		t.Errorf("totalPages = %d, want 1 for an empty result", none.Meta.TotalPages)
	}

	// A LIKE metacharacter is data, not syntax: it must not match everything.
	wild := f.list(t, f.workspace.ID.String(), "/api/products?search=%25")
	if len(wild.Data) != 0 {
		t.Errorf("searching for a literal %% returned %d products, want 0", len(wild.Data))
	}
}

func TestIntegrationPaginationRunsInPostgres(t *testing.T) {
	f := newAPIFixture(t)
	for _, name := range []string{"P1", "P2", "P3", "P4", "P5"} {
		f.createProduct(t, f.workspace, name, "10.00", "EUR", 1)
	}

	page1 := f.list(t, f.workspace.ID.String(), "/api/products?page=1&limit=2")
	page2 := f.list(t, f.workspace.ID.String(), "/api/products?page=2&limit=2")
	page3 := f.list(t, f.workspace.ID.String(), "/api/products?page=3&limit=2")

	if len(page1.Data) != 2 || len(page2.Data) != 2 || len(page3.Data) != 1 {
		t.Fatalf("page sizes = %d, %d, %d; want 2, 2, 1",
			len(page1.Data), len(page2.Data), len(page3.Data))
	}
	if page1.Meta.Total != 5 || page1.Meta.TotalPages != 3 {
		t.Errorf("meta = %+v, want total 5, totalPages 3", page1.Meta)
	}

	// Pages must not overlap, which is what the deterministic ORDER BY buys.
	seen := map[uuid.UUID]bool{}
	for _, page := range []listBody{page1, page2, page3} {
		for _, p := range page.Data {
			if seen[p.ID] {
				t.Errorf("product %s appeared on more than one page", p.ID)
			}
			seen[p.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("saw %d distinct products across the pages, want 5", len(seen))
	}

	// A page beyond the end is empty, not an error.
	beyond := f.list(t, f.workspace.ID.String(), "/api/products?page=99&limit=2")
	if len(beyond.Data) != 0 {
		t.Errorf("page 99 returned %d products, want 0", len(beyond.Data))
	}
	if beyond.Meta.Total != 5 {
		t.Errorf("page 99 total = %d, want 5", beyond.Meta.Total)
	}
}

// TestIntegrationWorkspaceIsolation is the tenancy guarantee: one workspace
// never sees another's catalogue, by listing, by search or by direct ID.
func TestIntegrationWorkspaceIsolation(t *testing.T) {
	f := newAPIFixture(t)

	mine := f.createProduct(t, f.workspace, "Mine Serum", "10.00", "EUR", 5)
	theirs := f.createProduct(t, f.other, "Their Serum", "20.00", "USD", 5)

	body := f.list(t, f.workspace.ID.String(), "/api/products")
	if len(body.Data) != 1 || body.Data[0].ID != mine.ID {
		t.Fatalf("workspace saw %d products, want only its own", len(body.Data))
	}
	if body.Meta.Total != 1 {
		t.Errorf("total = %d, want 1; the count must be scoped too", body.Meta.Total)
	}

	// Searching for the other workspace's product finds nothing.
	search := f.list(t, f.workspace.ID.String(), "/api/products?search=Their")
	if len(search.Data) != 0 {
		t.Errorf("search crossed into another workspace: %d results", len(search.Data))
	}

	// Fetching it by ID is 404, not 403: a different answer would confirm it
	// exists somewhere.
	res := f.get(t, f.workspace.ID.String(), "/api/products/"+theirs.ID.String())
	if res.Code != http.StatusNotFound {
		t.Errorf("cross-workspace fetch: status = %d, want 404", res.Code)
	}

	// And the owner still sees it.
	owner := f.get(t, f.other.ID.String(), "/api/products/"+theirs.ID.String())
	if owner.Code != http.StatusOK {
		t.Errorf("owner fetch: status = %d, want 200", owner.Code)
	}
}

func TestIntegrationWorkspaceHeaderIsRequired(t *testing.T) {
	f := newAPIFixture(t)
	f.createProduct(t, f.workspace, "Rose Serum", "10.00", "EUR", 5)

	if res := f.get(t, "", "/api/products"); res.Code != http.StatusBadRequest {
		t.Errorf("no header: status = %d, want 400", res.Code)
	}
	if res := f.get(t, "not-a-uuid", "/api/products"); res.Code != http.StatusBadRequest {
		t.Errorf("bad header: status = %d, want 400", res.Code)
	}
	if res := f.get(t, uuid.NewString(), "/api/products"); res.Code != http.StatusNotFound {
		t.Errorf("unknown workspace: status = %d, want 404", res.Code)
	}
}

func TestIntegrationInvalidPaginationIsRejected(t *testing.T) {
	f := newAPIFixture(t)

	for _, query := range []string{"?page=0", "?limit=0", "?limit=101", "?page=abc"} {
		res := f.get(t, f.workspace.ID.String(), "/api/products"+query)
		if res.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, res.Code)
		}
	}
}
