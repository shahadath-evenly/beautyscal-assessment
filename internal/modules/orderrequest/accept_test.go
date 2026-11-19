package orderrequest_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/modules/orderrequest"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
	"github.com/real-musafir/b-scale/backend/internal/platform/server"
	"github.com/real-musafir/b-scale/backend/internal/platform/validate"
)

// poolFixture runs against a real connection pool rather than a single shared
// transaction, because concurrency cannot be tested inside one transaction:
// two goroutines sharing a connection would serialise for the wrong reason.
// Each test gets its own workspaces and deletes them afterwards.
type poolFixture struct {
	router    http.Handler
	pool      *pgxpool.Pool
	queries   *sqlc.Queries
	ctx       context.Context
	workspace sqlc.Workspace
	other     sqlc.Workspace
}

func newPoolFixture(t *testing.T) *poolFixture {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("set DATABASE_URL or TEST_DATABASE_URL to run database tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	// Enough connections that concurrent accepts genuinely contend on the
	// row lock rather than queueing for a connection.
	cfg.MaxConns = 12

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	q := sqlc.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	f := &poolFixture{pool: pool, queries: q, ctx: ctx}
	f.router = server.NewRouter(server.Deps{
		Logger:     logger,
		Workspaces: q,
		OrderRequests: orderrequest.NewHandler(
			orderrequest.NewService(orderrequest.NewRepository(pool, q)),
			validate.New(),
			logger,
		),
		CORSAllowedOrigins: []string{"*"},
		RequestTimeout:     60 * time.Second,
	})

	f.workspace = f.createWorkspace(t, "accept-main")
	f.other = f.createWorkspace(t, "accept-other")
	return f
}

// createWorkspace makes an isolated workspace and schedules its removal.
// Orders reference requests with ON DELETE RESTRICT, so they are removed
// before the cascade from the workspace runs.
func (f *poolFixture) createWorkspace(t *testing.T, label string) sqlc.Workspace {
	t.Helper()

	ws, err := f.queries.CreateWorkspace(f.ctx, sqlc.CreateWorkspaceParams{
		Name: "Test " + label,
		Slug: label + "-" + uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		for _, stmt := range []string{
			`DELETE FROM order_items WHERE workspace_id = $1`,
			`DELETE FROM orders WHERE workspace_id = $1`,
			`DELETE FROM order_request_items WHERE workspace_id = $1`,
			`DELETE FROM order_requests WHERE workspace_id = $1`,
			`DELETE FROM products WHERE workspace_id = $1`,
			`DELETE FROM workspaces WHERE id = $1`,
		} {
			if _, err := f.pool.Exec(ctx, stmt, ws.ID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	return ws
}

func (f *poolFixture) createProduct(t *testing.T, ws sqlc.Workspace, name, price string, stock int32) sqlc.Product {
	t.Helper()
	amount, err := money.Parse(price)
	if err != nil {
		t.Fatalf("parse price: %v", err)
	}
	p, err := f.queries.CreateProduct(f.ctx, sqlc.CreateProductParams{
		WorkspaceID: ws.ID, Name: name, Price: amount, Currency: "EUR", Stock: stock,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return p
}

func (f *poolFixture) do(t *testing.T, method, target, workspaceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, f.router, method, target, workspaceID, body)
}

// createRequest posts a request and returns its ID.
func (f *poolFixture) createRequest(t *testing.T, ws sqlc.Workspace, items map[uuid.UUID]int32) uuid.UUID {
	t.Helper()

	res := f.do(t, http.MethodPost, "/api/order-requests", ws.ID.String(), createBody(items))
	if res.Code != http.StatusCreated {
		t.Fatalf("create request: status %d, body %s", res.Code, res.Body.String())
	}

	var body detailBody
	decodeInto(t, res, &body)
	return body.Data.ID
}

func (f *poolFixture) stockOf(t *testing.T, ws sqlc.Workspace, productID uuid.UUID) int32 {
	t.Helper()
	p, err := f.queries.GetProduct(f.ctx, sqlc.GetProductParams{WorkspaceID: ws.ID, ID: productID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	return p.Stock
}

func (f *poolFixture) orderCount(t *testing.T, ws sqlc.Workspace, requestID uuid.UUID) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx,
		`SELECT count(*) FROM orders WHERE workspace_id = $1 AND order_request_id = $2`,
		ws.ID, requestID).Scan(&count); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	return count
}

func (f *poolFixture) statusOf(t *testing.T, ws sqlc.Workspace, requestID uuid.UUID) string {
	t.Helper()
	r, err := f.queries.GetOrderRequest(f.ctx, sqlc.GetOrderRequestParams{WorkspaceID: ws.ID, ID: requestID})
	if err != nil {
		t.Fatalf("GetOrderRequest: %v", err)
	}
	return r.Status
}

// 1. Successful acceptance, and 2. stock reservation.
func TestAcceptReservesStockAndCreatesDraftOrder(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 10)
	mask := f.createProduct(t, f.workspace, "Clay Mask", "19.50", 5)

	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 3, mask.ID: 2})

	res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", res.Code, res.Body.String())
	}

	var body struct {
		Data orderrequest.AcceptResponse `json:"data"`
	}
	decodeInto(t, res, &body)

	if body.Data.Request.Status != "accepted" {
		t.Errorf("request status = %q, want accepted", body.Data.Request.Status)
	}
	if body.Data.Order.Status != "draft" {
		t.Errorf("order status = %q, want draft", body.Data.Order.Status)
	}
	if body.Data.Order.ID == uuid.Nil {
		t.Error("no order ID returned")
	}

	// Stock was reserved, exactly once and by the right amount.
	if got := f.stockOf(t, f.workspace, serum.ID); got != 7 {
		t.Errorf("serum stock = %d, want 7", got)
	}
	if got := f.stockOf(t, f.workspace, mask.ID); got != 3 {
		t.Errorf("mask stock = %d, want 3", got)
	}

	// 5. Exactly one order, with the snapshot prices copied onto it.
	if n := f.orderCount(t, f.workspace, requestID); n != 1 {
		t.Errorf("created %d orders, want 1", n)
	}

	lines, err := f.queries.ListOrderItems(f.ctx, sqlc.ListOrderItemsParams{
		WorkspaceID: f.workspace.ID, OrderID: body.Data.Order.ID,
	})
	if err != nil {
		t.Fatalf("ListOrderItems: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("order has %d lines, want 2", len(lines))
	}
	for _, line := range lines {
		if line.ProductID == serum.ID {
			if got := money.MustString(line.UnitPrice); got != "49.90" {
				t.Errorf("order line price = %q, want the quoted 49.90", got)
			}
			if line.Quantity != 3 {
				t.Errorf("order line quantity = %d, want 3", line.Quantity)
			}
		}
	}
}

// 3. Insufficient stock rolls everything back.
func TestAcceptWithInsufficientStockChangesNothing(t *testing.T) {
	f := newPoolFixture(t)
	// Plenty of the first product, not enough of the second: if the
	// implementation decremented as it went, the first would be left short.
	plenty := f.createProduct(t, f.workspace, "Plentiful Serum", "10.00", 100)
	scarce := f.createProduct(t, f.workspace, "Scarce Mask", "20.00", 2)

	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{plenty.ID: 5, scarce.ID: 3})

	res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", res.Code, res.Body.String())
	}

	var body errBody
	decodeInto(t, res, &body)
	if body.Error.Code != "insufficient_stock" {
		t.Errorf("code = %q, want insufficient_stock", body.Error.Code)
	}

	// No partial deduction, anywhere.
	if got := f.stockOf(t, f.workspace, plenty.ID); got != 100 {
		t.Errorf("plentiful stock = %d, want 100 untouched", got)
	}
	if got := f.stockOf(t, f.workspace, scarce.ID); got != 2 {
		t.Errorf("scarce stock = %d, want 2 untouched", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 0 {
		t.Errorf("created %d orders, want 0", n)
	}
	if status := f.statusOf(t, f.workspace, requestID); status != "new" {
		t.Errorf("status = %q, want new", status)
	}
}

// 4. A second acceptance is refused, and 5. only one order exists.
func TestAcceptTwiceIsRefused(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 10)
	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 4})

	first := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if first.Code != http.StatusOK {
		t.Fatalf("first accept: %d, body %s", first.Code, first.Body.String())
	}

	second := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if second.Code != http.StatusConflict {
		t.Fatalf("second accept: %d, want 409; body %s", second.Code, second.Body.String())
	}

	// Stock was taken once, not twice.
	if got := f.stockOf(t, f.workspace, serum.ID); got != 6 {
		t.Errorf("stock = %d, want 6 (one deduction only)", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 1 {
		t.Errorf("orders = %d, want 1", n)
	}
}

// 6. Concurrent acceptance: the heart of the exercise.
func TestConcurrentAcceptsProduceExactlyOneOrder(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 100)
	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 7})

	const attempts = 8

	var (
		start    sync.WaitGroup
		done     sync.WaitGroup
		mu       sync.Mutex
		statuses = map[int]int{}
	)
	start.Add(1)
	done.Add(attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			defer done.Done()
			// Release every goroutine at once, so they contend for real.
			start.Wait()

			res := doRequest(t, f.router, http.MethodPost,
				"/api/order-requests/"+requestID.String()+"/accept",
				f.workspace.ID.String(), "")

			mu.Lock()
			statuses[res.Code]++
			mu.Unlock()
		}()
	}

	start.Done()
	done.Wait()

	if statuses[http.StatusOK] != 1 {
		t.Errorf("%d acceptances succeeded, want exactly 1 (statuses: %v)", statuses[http.StatusOK], statuses)
	}
	if statuses[http.StatusConflict] != attempts-1 {
		t.Errorf("%d conflicts, want %d (statuses: %v)", statuses[http.StatusConflict], attempts-1, statuses)
	}
	if statuses[http.StatusInternalServerError] != 0 {
		t.Errorf("%d attempts failed with a server error; contention must not produce 500s", statuses[http.StatusInternalServerError])
	}

	// Stock decremented exactly once, one order, status accepted.
	if got := f.stockOf(t, f.workspace, serum.ID); got != 93 {
		t.Errorf("stock = %d, want 93 (100 - 7, deducted once)", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 1 {
		t.Errorf("orders = %d, want exactly 1", n)
	}
	if status := f.statusOf(t, f.workspace, requestID); status != "accepted" {
		t.Errorf("status = %q, want accepted", status)
	}
}

// Concurrent accepts of different requests competing for the same scarce
// stock: the database must not oversell.
func TestConcurrentAcceptsCannotOversell(t *testing.T) {
	f := newPoolFixture(t)
	// Five units, four requests wanting two each: at most two can succeed.
	serum := f.createProduct(t, f.workspace, "Scarce Serum", "10.00", 5)

	const requests = 4
	ids := make([]uuid.UUID, 0, requests)
	for i := 0; i < requests; i++ {
		ids = append(ids, f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 2}))
	}

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
		ok    int
	)
	start.Add(1)
	done.Add(requests)

	for _, id := range ids {
		go func(requestID uuid.UUID) {
			defer done.Done()
			start.Wait()

			res := doRequest(t, f.router, http.MethodPost,
				"/api/order-requests/"+requestID.String()+"/accept",
				f.workspace.ID.String(), "")

			mu.Lock()
			if res.Code == http.StatusOK {
				ok++
			}
			mu.Unlock()
		}(id)
	}

	start.Done()
	done.Wait()

	if ok != 2 {
		t.Errorf("%d acceptances succeeded, want 2 (5 units, 2 per request)", ok)
	}

	stock := f.stockOf(t, f.workspace, serum.ID)
	if stock < 0 {
		t.Fatalf("stock went negative: %d", stock)
	}
	if stock != 5-int32(ok*2) {
		t.Errorf("stock = %d, want %d", stock, 5-int32(ok*2))
	}
}

// 7. Another workspace cannot accept or reject this workspace's request.
func TestCrossWorkspaceAcceptIsRefused(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 10)
	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 3})

	// 404, not 403: a different answer would confirm the request exists.
	accept := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.other.ID.String(), "")
	if accept.Code != http.StatusNotFound {
		t.Errorf("cross-workspace accept: %d, want 404", accept.Code)
	}

	reject := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/reject",
		f.other.ID.String(), "")
	if reject.Code != http.StatusNotFound {
		t.Errorf("cross-workspace reject: %d, want 404", reject.Code)
	}

	// Nothing changed.
	if got := f.stockOf(t, f.workspace, serum.ID); got != 10 {
		t.Errorf("stock = %d, want 10 untouched", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 0 {
		t.Errorf("orders = %d, want 0", n)
	}
	if status := f.statusOf(t, f.workspace, requestID); status != "new" {
		t.Errorf("status = %q, want new", status)
	}

	// The owner can still accept it.
	owner := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if owner.Code != http.StatusOK {
		t.Errorf("owner accept: %d, want 200", owner.Code)
	}
}

// 8. Rejection, and the transitions it forbids.
func TestRejectDoesNotTouchStockOrCreateOrders(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 10)
	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 4})

	res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/reject",
		f.workspace.ID.String(), "")
	if res.Code != http.StatusOK {
		t.Fatalf("reject: %d, body %s", res.Code, res.Body.String())
	}

	var body detailBody
	decodeInto(t, res, &body)
	if body.Data.Status != "rejected" {
		t.Errorf("status = %q, want rejected", body.Data.Status)
	}

	if got := f.stockOf(t, f.workspace, serum.ID); got != 10 {
		t.Errorf("stock = %d, want 10 unchanged by a rejection", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 0 {
		t.Errorf("rejection created %d orders, want 0", n)
	}
}

func TestRejectedRequestCannotBeAccepted(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 10)
	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 4})

	if res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/reject",
		f.workspace.ID.String(), ""); res.Code != http.StatusOK {
		t.Fatalf("reject: %d", res.Code)
	}

	accept := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), "")
	if accept.Code != http.StatusConflict {
		t.Fatalf("accepting a rejected request: %d, want 409; body %s", accept.Code, accept.Body.String())
	}

	if got := f.stockOf(t, f.workspace, serum.ID); got != 10 {
		t.Errorf("stock = %d, want 10 untouched", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 0 {
		t.Errorf("orders = %d, want 0", n)
	}
	if status := f.statusOf(t, f.workspace, requestID); status != "rejected" {
		t.Errorf("status = %q, want rejected", status)
	}
}

func TestAcceptedRequestCannotBeRejected(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "49.90", 10)
	requestID := f.createRequest(t, f.workspace, map[uuid.UUID]int32{serum.ID: 4})

	if res := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/accept",
		f.workspace.ID.String(), ""); res.Code != http.StatusOK {
		t.Fatalf("accept: %d", res.Code)
	}

	reject := f.do(t, http.MethodPost, "/api/order-requests/"+requestID.String()+"/reject",
		f.workspace.ID.String(), "")
	if reject.Code != http.StatusConflict {
		t.Errorf("rejecting an accepted request: %d, want 409", reject.Code)
	}

	// The acceptance stands.
	if got := f.stockOf(t, f.workspace, serum.ID); got != 6 {
		t.Errorf("stock = %d, want 6", got)
	}
	if n := f.orderCount(t, f.workspace, requestID); n != 1 {
		t.Errorf("orders = %d, want 1", n)
	}
}

// A product listed twice is locked and decremented once, with the summed
// quantity.
func TestAcceptAggregatesRepeatedProducts(t *testing.T) {
	f := newPoolFixture(t)
	serum := f.createProduct(t, f.workspace, "Rose Serum", "10.00", 10)

	res := f.do(t, http.MethodPost, "/api/order-requests", f.workspace.ID.String(),
		fmt.Sprintf(`{"buyerName":"Ada","buyerEmail":"ada@example.com","items":[
			{"productId":%q,"quantity":2},{"productId":%q,"quantity":3}]}`, serum.ID, serum.ID))
	if res.Code != http.StatusCreated {
		t.Fatalf("create: %d, body %s", res.Code, res.Body.String())
	}
	var created detailBody
	decodeInto(t, res, &created)

	accept := f.do(t, http.MethodPost, "/api/order-requests/"+created.Data.ID.String()+"/accept",
		f.workspace.ID.String(), "")
	if accept.Code != http.StatusOK {
		t.Fatalf("accept: %d, body %s", accept.Code, accept.Body.String())
	}

	if got := f.stockOf(t, f.workspace, serum.ID); got != 5 {
		t.Errorf("stock = %d, want 5 (10 - 5 summed)", got)
	}
}

func TestAcceptUnknownRequest(t *testing.T) {
	f := newPoolFixture(t)

	if res := f.do(t, http.MethodPost, "/api/order-requests/"+uuid.NewString()+"/accept",
		f.workspace.ID.String(), ""); res.Code != http.StatusNotFound {
		t.Errorf("unknown request: %d, want 404", res.Code)
	}
	if res := f.do(t, http.MethodPost, "/api/order-requests/not-a-uuid/accept",
		f.workspace.ID.String(), ""); res.Code != http.StatusBadRequest {
		t.Errorf("malformed id: %d, want 400", res.Code)
	}
}
