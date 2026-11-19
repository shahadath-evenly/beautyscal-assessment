package orderrequest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/real-musafir/b-scale/backend/internal/modules/orderrequest"
	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
	"github.com/real-musafir/b-scale/backend/internal/platform/server"
	"github.com/real-musafir/b-scale/backend/internal/platform/validate"
)

type fixture struct {
	router    http.Handler
	queries   *sqlc.Queries
	ctx       context.Context
	workspace sqlc.Workspace
	other     sqlc.Workspace
}

// newFixture runs the real router, middleware and SQL inside a transaction
// that is always rolled back. The repository begins nested transactions
// against it, so the production code path is exercised unchanged.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("set DATABASE_URL or TEST_DATABASE_URL to run database tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	f := &fixture{queries: q, ctx: ctx}
	f.router = server.NewRouter(server.Deps{
		Logger:     logger,
		Workspaces: q,
		OrderRequests: orderrequest.NewHandler(
			orderrequest.NewService(orderrequest.NewRepository(tx, q)),
			validate.New(),
			logger,
		),
		CORSAllowedOrigins: []string{"*"},
		RequestTimeout:     30 * time.Second,
	})

	f.workspace = f.createWorkspace(t, "req-main")
	f.other = f.createWorkspace(t, "req-other")
	return f
}

func (f *fixture) createWorkspace(t *testing.T, label string) sqlc.Workspace {
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

func (f *fixture) createProduct(t *testing.T, ws sqlc.Workspace, name, price, currency string) sqlc.Product {
	t.Helper()
	amount, err := money.Parse(price)
	if err != nil {
		t.Fatalf("parse price: %v", err)
	}
	p, err := f.queries.CreateProduct(f.ctx, sqlc.CreateProductParams{
		WorkspaceID: ws.ID, Name: name, Price: amount, Currency: currency, Stock: 100,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return p
}

func (f *fixture) do(t *testing.T, method, target, workspaceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, f.router, method, target, workspaceID, body)
}

// doRequest issues one request through a router, as a workspace. Safe to call
// from several goroutines: each call builds its own request and recorder.
func doRequest(t *testing.T, router http.Handler, method, target, workspaceID, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if workspaceID != "" {
		req.Header.Set("X-Workspace-ID", workspaceID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

type detailBody struct {
	Data orderrequest.DetailResponse `json:"data"`
}

type listBody struct {
	Data []orderrequest.Response `json:"data"`
	Meta httpx.Meta              `json:"meta"`
}

type errBody struct {
	Error struct {
		Code    string             `json:"code"`
		Message string             `json:"message"`
		Details []httpx.FieldError `json:"details"`
	} `json:"error"`
}

func decodeInto(t *testing.T, res *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(res.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode %q: %v", res.Body.String(), err)
	}
}

// createBody builds a valid payload for the given product IDs and quantities.
func createBody(items map[uuid.UUID]int32) string {
	lines := make([]string, 0, len(items))
	for id, quantity := range items {
		lines = append(lines, fmt.Sprintf(`{"productId":%q,"quantity":%d}`, id, quantity))
	}
	body := `{"buyerName":"Ada Lovelace","buyerEmail":"ada@example.com","items":[`
	for i, line := range lines {
		if i > 0 {
			body += ","
		}
		body += line
	}
	return body + `]}`
}

func TestCreateOrderRequest(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")
	mask := f.createProduct(t, f.workspace, "Clay Mask", "19.50", "EUR")

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		fmt.Sprintf(`{"buyerName":"Ada Lovelace","buyerEmail":"ada@example.com","items":[
			{"productId":%q,"quantity":2},{"productId":%q,"quantity":1}]}`, serum.ID, mask.ID))

	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", res.Code, res.Body.String())
	}

	var body detailBody
	decodeInto(t, res, &body)
	detail := body.Data

	if detail.Status != "new" {
		t.Errorf("status = %q, want new", detail.Status)
	}
	if detail.BuyerName != "Ada Lovelace" || detail.BuyerEmail != "ada@example.com" {
		t.Errorf("buyer = %q / %q", detail.BuyerName, detail.BuyerEmail)
	}
	if len(detail.Items) != 2 {
		t.Fatalf("returned %d lines, want 2", len(detail.Items))
	}
	// 2 x 49.90 + 1 x 19.50
	if detail.Total != "119.30" {
		t.Errorf("total = %q, want 119.30", detail.Total)
	}
	if detail.Currency != "EUR" {
		t.Errorf("currency = %q, want EUR", detail.Currency)
	}

	for _, item := range detail.Items {
		if item.ProductID == serum.ID {
			if item.UnitPrice != "49.90" || item.LineTotal != "99.80" {
				t.Errorf("serum line = %+v, want unit 49.90 and total 99.80", item)
			}
			if item.ProductName != "Rose Serum" {
				t.Errorf("product name snapshot = %q", item.ProductName)
			}
		}
	}
}

// TestCreateIgnoresClientSuppliedPrices is the rule that a client cannot name
// its own price: an unknown field is rejected outright, so the payload has no
// way to carry one.
func TestCreateIgnoresClientSuppliedPrices(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"ada@example.com","items":[
			{"productId":%q,"quantity":1,"unitPrice":"0.01"}]}`, serum.ID))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field; body %s", res.Code, res.Body.String())
	}

	// And nothing was stored.
	list := f.do(t, http.MethodGet, "/api/order-requests", f.workspace.ID.String(), "")
	var body listBody
	decodeInto(t, list, &body)
	if body.Meta.Total != 0 {
		t.Errorf("a rejected request was stored: total = %d", body.Meta.Total)
	}
}

// TestCreateSnapshotsSurvivePriceChanges proves the snapshot is a copy, not a
// reference: changing the catalogue afterwards leaves the request untouched.
func TestCreateSnapshotsSurvivePriceChanges(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		createBody(map[uuid.UUID]int32{serum.ID: 2}))
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.Code)
	}
	var created detailBody
	decodeInto(t, res, &created)

	newPrice, err := money.Parse("99.99")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := f.queries.UpdateProductCatalogue(f.ctx, sqlc.UpdateProductCatalogueParams{
		WorkspaceID: f.workspace.ID, ID: serum.ID, Price: newPrice, Currency: "EUR", Stock: 50,
	}); err != nil {
		t.Fatalf("UpdateProductCatalogue: %v", err)
	}

	fetched := f.do(t, http.MethodGet, "/api/order-requests/"+created.Data.ID.String(),
		f.workspace.ID.String(), "")
	var after detailBody
	decodeInto(t, fetched, &after)

	if after.Data.Items[0].UnitPrice != "49.90" {
		t.Errorf("unit price = %q, want the 49.90 quoted at creation", after.Data.Items[0].UnitPrice)
	}
	if after.Data.Total != "99.80" {
		t.Errorf("total = %q, want 99.80", after.Data.Total)
	}
}

func TestCreateValidation(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"missing buyer name", fmt.Sprintf(`{"buyerName":"","buyerEmail":"a@b.com","items":[{"productId":%q,"quantity":1}]}`, serum.ID), http.StatusUnprocessableEntity},
		{"invalid email", fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"not-an-email","items":[{"productId":%q,"quantity":1}]}`, serum.ID), http.StatusUnprocessableEntity},
		{"no items", `{"buyerName":"Ada","buyerEmail":"a@b.com","items":[]}`, http.StatusUnprocessableEntity},
		{"missing items", `{"buyerName":"Ada","buyerEmail":"a@b.com"}`, http.StatusUnprocessableEntity},
		{"zero quantity", fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"a@b.com","items":[{"productId":%q,"quantity":0}]}`, serum.ID), http.StatusUnprocessableEntity},
		{"negative quantity", fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"a@b.com","items":[{"productId":%q,"quantity":-3}]}`, serum.ID), http.StatusUnprocessableEntity},
		{"unknown product", fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"a@b.com","items":[{"productId":%q,"quantity":1}]}`, uuid.New()), http.StatusUnprocessableEntity},
		{"malformed json", `{"buyerName":`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(), tc.body)
			if res.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", res.Code, tc.wantStatus, res.Body.String())
			}

			var body errBody
			decodeInto(t, res, &body)
			if body.Error.Code == "" {
				t.Error("error envelope has no code")
			}
		})
	}

	// None of the rejected payloads created anything.
	list := f.do(t, http.MethodGet, "/api/order-requests", f.workspace.ID.String(), "")
	var body listBody
	decodeInto(t, list, &body)
	if body.Meta.Total != 0 {
		t.Errorf("%d requests were stored despite validation failures", body.Meta.Total)
	}
}

// TestCreateRejectsForeignProduct: a product that exists, but in another
// workspace, is reported exactly like one that does not exist.
func TestCreateRejectsForeignProduct(t *testing.T) {
	f := newFixture(t)
	foreign := f.createProduct(t, f.other, "Their Serum", "10.00", "USD")

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		createBody(map[uuid.UUID]int32{foreign.ID: 1}))

	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", res.Code, res.Body.String())
	}

	var body errBody
	decodeInto(t, res, &body)
	if len(body.Error.Details) == 0 {
		t.Error("expected details naming the offending product")
	}
}

func TestCreateRejectsMixedCurrencies(t *testing.T) {
	f := newFixture(t)
	eur := f.createProduct(t, f.workspace, "Euro Serum", "49.90", "EUR")
	usd := f.createProduct(t, f.workspace, "Dollar Mask", "19.50", "USD")

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		createBody(map[uuid.UUID]int32{eur.ID: 1, usd.ID: 1}))

	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", res.Code, res.Body.String())
	}

	var body errBody
	decodeInto(t, res, &body)
	if len(body.Error.Details) == 0 {
		t.Fatal("expected details explaining the currency mismatch")
	}

	// Nothing partial survived the rolled-back transaction.
	list := f.do(t, http.MethodGet, "/api/order-requests", f.workspace.ID.String(), "")
	var listed listBody
	decodeInto(t, list, &listed)
	if listed.Meta.Total != 0 {
		t.Errorf("a request survived a mixed-currency failure: total = %d", listed.Meta.Total)
	}
}

func TestCreateMergesRepeatedProducts(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "10.00", "EUR")

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"ada@example.com","items":[
			{"productId":%q,"quantity":2},{"productId":%q,"quantity":3}]}`, serum.ID, serum.ID))

	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", res.Code, res.Body.String())
	}

	var body detailBody
	decodeInto(t, res, &body)

	if len(body.Data.Items) != 1 {
		t.Fatalf("stored %d lines, want 1 merged line", len(body.Data.Items))
	}
	if body.Data.Items[0].Quantity != 5 {
		t.Errorf("quantity = %d, want 5", body.Data.Items[0].Quantity)
	}
	if body.Data.Total != "50.00" {
		t.Errorf("total = %q, want 50.00", body.Data.Total)
	}
}

func TestListFilterSearchAndPaginate(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "10.00", "EUR")

	for _, buyer := range []struct{ name, email string }{
		{"Ada Lovelace", "ada@example.com"},
		{"Grace Hopper", "grace@example.com"},
		{"Alan Turing", "alan@example.com"},
	} {
		body := fmt.Sprintf(`{"buyerName":%q,"buyerEmail":%q,"items":[{"productId":%q,"quantity":1}]}`,
			buyer.name, buyer.email, serum.ID)
		if res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(), body); res.Code != http.StatusCreated {
			t.Fatalf("seed %s: status %d, body %s", buyer.name, res.Code, res.Body.String())
		}
	}

	all := f.do(t, http.MethodGet, "/api/order-requests", f.workspace.ID.String(), "")
	var body listBody
	decodeInto(t, all, &body)
	if body.Meta.Total != 3 {
		t.Fatalf("total = %d, want 3", body.Meta.Total)
	}
	// Listings carry the totals the sales table shows.
	for _, r := range body.Data {
		if r.Total != "10.00" || r.Currency != "EUR" || r.ItemCount != 1 {
			t.Errorf("summary = %+v, want total 10.00 EUR and one item", r)
		}
	}

	// Status filter.
	newOnes := f.do(t, http.MethodGet, "/api/order-requests?status=new", f.workspace.ID.String(), "")
	decodeInto(t, newOnes, &body)
	if body.Meta.Total != 3 {
		t.Errorf("status=new total = %d, want 3", body.Meta.Total)
	}

	accepted := f.do(t, http.MethodGet, "/api/order-requests?status=accepted", f.workspace.ID.String(), "")
	decodeInto(t, accepted, &body)
	if body.Meta.Total != 0 || len(body.Data) != 0 {
		t.Errorf("status=accepted returned %d requests, want 0", len(body.Data))
	}

	// An unsupported status is a 400 naming the field.
	bad := f.do(t, http.MethodGet, "/api/order-requests?status=cancelled", f.workspace.ID.String(), "")
	if bad.Code != http.StatusBadRequest {
		t.Errorf("status=cancelled: %d, want 400", bad.Code)
	}

	// Search covers buyer name and email.
	byName := f.do(t, http.MethodGet, "/api/order-requests?search=grace", f.workspace.ID.String(), "")
	decodeInto(t, byName, &body)
	if body.Meta.Total != 1 || body.Data[0].BuyerName != "Grace Hopper" {
		t.Errorf("search by name returned %+v", body.Data)
	}

	byEmail := f.do(t, http.MethodGet, "/api/order-requests?search=alan@", f.workspace.ID.String(), "")
	decodeInto(t, byEmail, &body)
	if body.Meta.Total != 1 || body.Data[0].BuyerEmail != "alan@example.com" {
		t.Errorf("search by email returned %+v", body.Data)
	}

	// Pagination runs in SQL and pages do not overlap. Each page is decoded
	// into its own value: unmarshalling into a shared struct would reuse the
	// slice's backing array and make page one alias page two.
	var firstPage, secondPage listBody

	decodeInto(t, f.do(t, http.MethodGet, "/api/order-requests?page=1&limit=2", f.workspace.ID.String(), ""), &firstPage)
	if len(firstPage.Data) != 2 || firstPage.Meta.TotalPages != 2 {
		t.Fatalf("page 1 = %d items, totalPages %d; want 2 and 2", len(firstPage.Data), firstPage.Meta.TotalPages)
	}

	decodeInto(t, f.do(t, http.MethodGet, "/api/order-requests?page=2&limit=2", f.workspace.ID.String(), ""), &secondPage)
	if len(secondPage.Data) != 1 {
		t.Fatalf("page 2 = %d items, want 1", len(secondPage.Data))
	}
	for _, a := range firstPage.Data {
		if a.ID == secondPage.Data[0].ID {
			t.Errorf("request %s appeared on both pages", a.ID)
		}
	}
}

func TestGetDetail(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", "EUR")
	mask := f.createProduct(t, f.workspace, "Clay Mask", "19.50", "EUR")

	created := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"ada@example.com","items":[
			{"productId":%q,"quantity":2},{"productId":%q,"quantity":4}]}`, serum.ID, mask.ID))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d, body %s", created.Code, created.Body.String())
	}
	var createdBody detailBody
	decodeInto(t, created, &createdBody)

	res := f.do(t, http.MethodGet, "/api/order-requests/"+createdBody.Data.ID.String(),
		f.workspace.ID.String(), "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}

	var body detailBody
	decodeInto(t, res, &body)
	detail := body.Data

	if detail.BuyerName != "Ada" || detail.Status != "new" {
		t.Errorf("detail = %+v", detail)
	}
	if detail.CreatedAt.IsZero() {
		t.Error("createdAt is missing")
	}
	if len(detail.Items) != 2 || detail.ItemCount != 2 {
		t.Fatalf("returned %d lines", len(detail.Items))
	}
	// 2 x 49.90 + 4 x 19.50 = 99.80 + 78.00
	if detail.Total != "177.80" || detail.Currency != "EUR" {
		t.Errorf("total = %q %q, want 177.80 EUR", detail.Total, detail.Currency)
	}

	// A malformed ID is a 400; an unknown one a 404.
	if bad := f.do(t, http.MethodGet, "/api/order-requests/not-a-uuid", f.workspace.ID.String(), ""); bad.Code != http.StatusBadRequest {
		t.Errorf("malformed id: %d, want 400", bad.Code)
	}
	if missing := f.do(t, http.MethodGet, "/api/order-requests/"+uuid.NewString(), f.workspace.ID.String(), ""); missing.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d, want 404", missing.Code)
	}
}

// TestWorkspaceIsolation: a request created in one workspace is invisible to
// the other, by listing, by search and by direct ID.
func TestWorkspaceIsolation(t *testing.T) {
	f := newFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "10.00", "EUR")

	created := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		createBody(map[uuid.UUID]int32{serum.ID: 1}))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d", created.Code)
	}
	var body detailBody
	decodeInto(t, created, &body)

	// The other workspace sees nothing.
	list := f.do(t, http.MethodGet, "/api/order-requests", f.other.ID.String(), "")
	var listed listBody
	decodeInto(t, list, &listed)
	if listed.Meta.Total != 0 || len(listed.Data) != 0 {
		t.Errorf("other workspace saw %d requests, want 0", len(listed.Data))
	}

	// Searching for the buyer finds nothing either.
	search := f.do(t, http.MethodGet, "/api/order-requests?search=Ada", f.other.ID.String(), "")
	decodeInto(t, search, &listed)
	if listed.Meta.Total != 0 {
		t.Errorf("search crossed workspaces: %d results", listed.Meta.Total)
	}

	// Fetching by ID is 404, indistinguishable from a request that never
	// existed.
	foreign := f.do(t, http.MethodGet, "/api/order-requests/"+body.Data.ID.String(), f.other.ID.String(), "")
	if foreign.Code != http.StatusNotFound {
		t.Errorf("cross-workspace fetch: %d, want 404", foreign.Code)
	}

	// The owner still sees it.
	owner := f.do(t, http.MethodGet, "/api/order-requests/"+body.Data.ID.String(), f.workspace.ID.String(), "")
	if owner.Code != http.StatusOK {
		t.Errorf("owner fetch: %d, want 200", owner.Code)
	}
}

func TestEndpointsRequireAWorkspaceHeader(t *testing.T) {
	f := newFixture(t)

	for _, target := range []string{"/api/order-requests", "/api/order-requests/" + uuid.NewString()} {
		if res := f.do(t, http.MethodGet, target, "", ""); res.Code != http.StatusBadRequest {
			t.Errorf("GET %s without a header: %d, want 400", target, res.Code)
		}
	}
	if res := f.do(t, http.MethodPost, "/api/order-requests", "", `{"buyerName":"Ada"}`); res.Code != http.StatusBadRequest {
		t.Errorf("POST without a header: %d, want 400", res.Code)
	}
}
