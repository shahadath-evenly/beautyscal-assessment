package sqlc_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// withTx gives each test a transaction that is always rolled back, so the
// generated queries are exercised against a real PostgreSQL with real
// constraints while leaving the database exactly as it was found.
func withTx(t *testing.T) (context.Context, *sqlc.Queries) {
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

	return ctx, sqlc.New(tx)
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("scan numeric %q: %v", s, err)
	}
	return n
}

func numericString(t *testing.T, n pgtype.Numeric) string {
	t.Helper()
	v, err := n.Value()
	if err != nil {
		t.Fatalf("numeric value: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("numeric value is %T, want string", v)
	}
	return s
}

// seedWorkspace creates a workspace with a unique slug, so parallel runs and
// repeated runs never collide.
func seedWorkspace(ctx context.Context, t *testing.T, q *sqlc.Queries, label string) sqlc.Workspace {
	t.Helper()
	slug := label + "-" + uuid.NewString()[:8]
	ws, err := q.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{Name: "Test " + label, Slug: slug})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return ws
}

func seedProduct(ctx context.Context, t *testing.T, q *sqlc.Queries, wsID uuid.UUID, name, price string, stock int32) sqlc.Product {
	t.Helper()
	p, err := q.CreateProduct(ctx, sqlc.CreateProductParams{
		WorkspaceID: wsID,
		Name:        name,
		Price:       numeric(t, price),
		Currency:    "EUR",
		Stock:       stock,
	})
	if err != nil {
		t.Fatalf("CreateProduct(%s): %v", name, err)
	}
	return p
}

func TestWorkspaceQueries(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "alpha")

	got, err := q.GetWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got.Slug != ws.Slug {
		t.Errorf("slug = %q, want %q", got.Slug, ws.Slug)
	}

	exists, err := q.WorkspaceExists(ctx, ws.ID)
	if err != nil || !exists {
		t.Fatalf("WorkspaceExists = %v, %v; want true, nil", exists, err)
	}
	// An unknown workspace must report false rather than erroring, so the
	// middleware can answer 400/404 without special-casing no-rows.
	if exists, err := q.WorkspaceExists(ctx, uuid.New()); err != nil || exists {
		t.Fatalf("WorkspaceExists(unknown) = %v, %v; want false, nil", exists, err)
	}
}

func TestProductListingSearchAndPagination(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "catalogue")

	for _, name := range []string{"Rose Serum", "Rose Cream", "Clay Mask"} {
		seedProduct(ctx, t, q, ws.ID, name, "19.99", 10)
	}

	// Search runs in SQL and is parameterised.
	search := "rose"
	found, err := q.ListProducts(ctx, sqlc.ListProductsParams{
		WorkspaceID: ws.ID, Search: &search, PageLimit: 10, PageOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("search %q returned %d products, want 2", search, len(found))
	}

	total, err := q.CountProducts(ctx, sqlc.CountProductsParams{WorkspaceID: ws.ID, Search: &search})
	if err != nil {
		t.Fatalf("CountProducts: %v", err)
	}
	if total != 2 {
		t.Errorf("CountProducts = %d, want 2", total)
	}

	// Pagination happens in PostgreSQL: page 2 of size 2 holds the remaining
	// row and does not repeat page 1.
	page1, err := q.ListProducts(ctx, sqlc.ListProductsParams{WorkspaceID: ws.ID, PageLimit: 2, PageOffset: 0})
	if err != nil {
		t.Fatalf("ListProducts page 1: %v", err)
	}
	page2, err := q.ListProducts(ctx, sqlc.ListProductsParams{WorkspaceID: ws.ID, PageLimit: 2, PageOffset: 2})
	if err != nil {
		t.Fatalf("ListProducts page 2: %v", err)
	}
	if len(page1) != 2 || len(page2) != 1 {
		t.Fatalf("page sizes = %d, %d; want 2, 1", len(page1), len(page2))
	}
	for _, a := range page1 {
		for _, b := range page2 {
			if a.ID == b.ID {
				t.Errorf("product %s appears on both pages", a.ID)
			}
		}
	}

	// A nil search must not filter anything out.
	all, err := q.ListProducts(ctx, sqlc.ListProductsParams{WorkspaceID: ws.ID, PageLimit: 10, PageOffset: 0})
	if err != nil {
		t.Fatalf("ListProducts(all): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("unfiltered list returned %d, want 3", len(all))
	}
}

func TestTenantIsolationInQueries(t *testing.T) {
	ctx, q := withTx(t)
	wsA := seedWorkspace(ctx, t, q, "tenant-a")
	wsB := seedWorkspace(ctx, t, q, "tenant-b")

	productA := seedProduct(ctx, t, q, wsA.ID, "A Serum", "10.00", 5)

	// Workspace B must not be able to read workspace A's product.
	if _, err := q.GetProduct(ctx, sqlc.GetProductParams{WorkspaceID: wsB.ID, ID: productA.ID}); err == nil {
		t.Error("GetProduct returned another workspace's product")
	}

	requestA, err := q.CreateOrderRequest(ctx, sqlc.CreateOrderRequestParams{
		WorkspaceID: wsA.ID, BuyerName: "Buyer", BuyerEmail: "buyer@example.com",
	})
	if err != nil {
		t.Fatalf("CreateOrderRequest: %v", err)
	}

	// Nor read, accept or reject workspace A's request.
	if _, err := q.GetOrderRequest(ctx, sqlc.GetOrderRequestParams{WorkspaceID: wsB.ID, ID: requestA.ID}); err == nil {
		t.Error("GetOrderRequest returned another workspace's request")
	}
	if _, err := q.MarkOrderRequestAccepted(ctx, sqlc.MarkOrderRequestAcceptedParams{WorkspaceID: wsB.ID, ID: requestA.ID}); err == nil {
		t.Error("MarkOrderRequestAccepted accepted another workspace's request")
	}
	if _, err := q.MarkOrderRequestRejected(ctx, sqlc.MarkOrderRequestRejectedParams{WorkspaceID: wsB.ID, ID: requestA.ID}); err == nil {
		t.Error("MarkOrderRequestRejected rejected another workspace's request")
	}
}

// TestAcceptFlowStatements walks the exact statement sequence the accept
// transaction will use, confirming the generated queries compose correctly.
func TestAcceptFlowStatements(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "accept")
	serum := seedProduct(ctx, t, q, ws.ID, "Rose Serum", "49.90", 10)
	mask := seedProduct(ctx, t, q, ws.ID, "Clay Mask", "19.50", 3)

	request, err := q.CreateOrderRequest(ctx, sqlc.CreateOrderRequestParams{
		WorkspaceID: ws.ID, BuyerName: "Buyer One", BuyerEmail: "buyer@example.com",
	})
	if err != nil {
		t.Fatalf("CreateOrderRequest: %v", err)
	}

	// Bulk insert of the lines through COPY.
	inserted, err := q.CreateOrderRequestItems(ctx, []sqlc.CreateOrderRequestItemsParams{
		{WorkspaceID: ws.ID, OrderRequestID: request.ID, ProductID: serum.ID,
			ProductName: serum.Name, Quantity: 2, UnitPrice: serum.Price, Currency: serum.Currency},
		{WorkspaceID: ws.ID, OrderRequestID: request.ID, ProductID: mask.ID,
			ProductName: mask.Name, Quantity: 1, UnitPrice: mask.Price, Currency: mask.Currency},
	})
	if err != nil {
		t.Fatalf("CreateOrderRequestItems: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("copied %d rows, want 2", inserted)
	}

	// Step 1: lock the request.
	locked, err := q.GetOrderRequestForUpdate(ctx, sqlc.GetOrderRequestForUpdateParams{WorkspaceID: ws.ID, ID: request.ID})
	if err != nil {
		t.Fatalf("GetOrderRequestForUpdate: %v", err)
	}
	if locked.Status != "new" {
		t.Fatalf("status = %q, want new", locked.Status)
	}

	// Step 2: aggregate by product, in deterministic lock order.
	totals, err := q.SumOrderRequestQuantitiesByProduct(ctx, sqlc.SumOrderRequestQuantitiesByProductParams{
		WorkspaceID: ws.ID, OrderRequestID: request.ID,
	})
	if err != nil {
		t.Fatalf("SumOrderRequestQuantitiesByProduct: %v", err)
	}
	if len(totals) != 2 {
		t.Fatalf("aggregated into %d products, want 2", len(totals))
	}

	ids := make([]uuid.UUID, 0, len(totals))
	for _, row := range totals {
		ids = append(ids, row.ProductID)
	}

	// Step 3: lock the products.
	lockedProducts, err := q.LockProductsForUpdate(ctx, sqlc.LockProductsForUpdateParams{
		WorkspaceID: ws.ID, ProductIds: ids,
	})
	if err != nil {
		t.Fatalf("LockProductsForUpdate: %v", err)
	}
	if len(lockedProducts) != 2 {
		t.Fatalf("locked %d products, want 2", len(lockedProducts))
	}

	// Step 4: decrement stock.
	for _, row := range totals {
		updated, err := q.DecrementProductStock(ctx, sqlc.DecrementProductStockParams{
			WorkspaceID: ws.ID, ID: row.ProductID, Quantity: row.TotalQuantity,
		})
		if err != nil {
			t.Fatalf("DecrementProductStock(%s): %v", row.ProductID, err)
		}
		if updated.Stock < 0 {
			t.Errorf("stock went negative: %d", updated.Stock)
		}
	}

	after, err := q.GetProduct(ctx, sqlc.GetProductParams{WorkspaceID: ws.ID, ID: serum.ID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if after.Stock != 8 {
		t.Errorf("serum stock = %d, want 8", after.Stock)
	}

	// Step 5: the request transitions exactly once.
	if _, err := q.MarkOrderRequestAccepted(ctx, sqlc.MarkOrderRequestAcceptedParams{WorkspaceID: ws.ID, ID: request.ID}); err != nil {
		t.Fatalf("MarkOrderRequestAccepted: %v", err)
	}
	// A second attempt matches no row, because status is no longer 'new'.
	if _, err := q.MarkOrderRequestAccepted(ctx, sqlc.MarkOrderRequestAcceptedParams{WorkspaceID: ws.ID, ID: request.ID}); err == nil {
		t.Error("second accept updated a row; the status guard did not hold")
	}

	// Step 6: one draft order, with lines copied from the request snapshots.
	order, err := q.CreateOrder(ctx, sqlc.CreateOrderParams{WorkspaceID: ws.ID, OrderRequestID: request.ID})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Status != "draft" {
		t.Errorf("order status = %q, want draft", order.Status)
	}

	lines, err := q.CopyOrderItemsFromRequest(ctx, sqlc.CopyOrderItemsFromRequestParams{
		WorkspaceID: ws.ID, OrderID: order.ID, OrderRequestID: request.ID,
	})
	if err != nil {
		t.Fatalf("CopyOrderItemsFromRequest: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("copied %d order lines, want 2", len(lines))
	}

	// Money must survive the round trip exactly.
	for _, line := range lines {
		if line.ProductID == serum.ID && numericString(t, line.UnitPrice) != "49.90" {
			t.Errorf("unit price = %q, want 49.90", numericString(t, line.UnitPrice))
		}
	}

	// A second order for the same request violates the unique constraint.
	if _, err := q.CreateOrder(ctx, sqlc.CreateOrderParams{WorkspaceID: ws.ID, OrderRequestID: request.ID}); err == nil {
		t.Error("a second order was created for the same request")
	}
}

func TestDecrementRefusesOversell(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "oversell")
	product := seedProduct(ctx, t, q, ws.ID, "Scarce Serum", "10.00", 2)

	// The stock guard lives in the WHERE clause, so an oversell updates no
	// row and returns pgx.ErrNoRows rather than writing a negative stock.
	if _, err := q.DecrementProductStock(ctx, sqlc.DecrementProductStockParams{
		WorkspaceID: ws.ID, ID: product.ID, Quantity: 3,
	}); err == nil {
		t.Fatal("oversell succeeded; stock guard did not hold")
	}

	unchanged, err := q.GetProduct(ctx, sqlc.GetProductParams{WorkspaceID: ws.ID, ID: product.ID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if unchanged.Stock != 2 {
		t.Errorf("stock = %d, want 2 unchanged", unchanged.Stock)
	}
}

func TestOrderRequestListingFilters(t *testing.T) {
	ctx, q := withTx(t)
	ws := seedWorkspace(ctx, t, q, "requests")

	for _, buyer := range []struct{ name, email string }{
		{"Ada Lovelace", "ada@example.com"},
		{"Grace Hopper", "grace@example.com"},
	} {
		if _, err := q.CreateOrderRequest(ctx, sqlc.CreateOrderRequestParams{
			WorkspaceID: ws.ID, BuyerName: buyer.name, BuyerEmail: buyer.email,
		}); err != nil {
			t.Fatalf("CreateOrderRequest: %v", err)
		}
	}

	status := "new"
	byStatus, err := q.ListOrderRequests(ctx, sqlc.ListOrderRequestsParams{
		WorkspaceID: ws.ID, Status: &status, PageLimit: 10, PageOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListOrderRequests: %v", err)
	}
	if len(byStatus) != 2 {
		t.Errorf("status filter returned %d, want 2", len(byStatus))
	}

	// Search covers buyer name and email.
	search := "grace@"
	byEmail, err := q.ListOrderRequests(ctx, sqlc.ListOrderRequestsParams{
		WorkspaceID: ws.ID, Search: &search, PageLimit: 10, PageOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListOrderRequests(search): %v", err)
	}
	if len(byEmail) != 1 {
		t.Fatalf("email search returned %d, want 1", len(byEmail))
	}

	count, err := q.CountOrderRequests(ctx, sqlc.CountOrderRequestsParams{WorkspaceID: ws.ID, Status: &status})
	if err != nil {
		t.Fatalf("CountOrderRequests: %v", err)
	}
	if count != 2 {
		t.Errorf("CountOrderRequests = %d, want 2", count)
	}

	// A status that matches nothing must return an empty slice, not nil, so
	// the API never serialises "null" for a list.
	missing := "rejected"
	none, err := q.ListOrderRequests(ctx, sqlc.ListOrderRequestsParams{
		WorkspaceID: ws.ID, Status: &missing, PageLimit: 10, PageOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListOrderRequests(rejected): %v", err)
	}
	if none == nil {
		t.Error("empty result was nil; want an empty slice")
	}
}
