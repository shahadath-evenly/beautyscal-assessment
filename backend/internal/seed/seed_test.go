package seed_test

import (
	"context"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/seed"
)

// withTx runs each test inside a transaction that is always rolled back, so
// the seed is exercised against real constraints without leaving demo rows
// behind.
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

func TestSeedIsIdempotent(t *testing.T) {
	ctx, q := withTx(t)

	first, err := seed.Run(ctx, q, seed.Options{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(first) != len(seed.Demo) {
		t.Fatalf("seeded %d workspaces, want %d", len(first), len(seed.Demo))
	}

	second, err := seed.Run(ctx, q, seed.Options{})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	for i, before := range first {
		after := second[i]

		// Identity must survive a re-run, or anything referencing a
		// workspace by ID would break.
		if before.ID != after.ID {
			t.Errorf("%s: workspace ID changed from %s to %s", before.Slug, before.ID, after.ID)
		}
		if before.Total != after.Total {
			t.Errorf("%s: product count changed from %d to %d", before.Slug, before.Total, after.Total)
		}
		if after.Created != 0 || after.Updated != 0 {
			t.Errorf("%s: second run created %d and updated %d rows; want 0 and 0",
				before.Slug, after.Created, after.Updated)
		}
		if after.Existing != len(seed.Demo[i].Products) {
			t.Errorf("%s: second run saw %d existing products, want %d",
				before.Slug, after.Existing, len(seed.Demo[i].Products))
		}
	}
}

func TestSeedScopesProductsToTheirWorkspace(t *testing.T) {
	ctx, q := withTx(t)

	results, err := seed.Run(ctx, q, seed.Options{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i, result := range results {
		want := seed.Demo[i]

		if result.Total != len(want.Products) {
			t.Errorf("%s: %d products, want %d", want.Slug, result.Total, len(want.Products))
		}

		products, err := q.ListProducts(ctx, sqlc.ListProductsParams{
			WorkspaceID: result.ID, PageLimit: 100, PageOffset: 0,
		})
		if err != nil {
			t.Fatalf("ListProducts(%s): %v", want.Slug, err)
		}

		names := make(map[string]int, len(products))
		for _, p := range products {
			// Every product must carry the workspace it was seeded into.
			if p.WorkspaceID != result.ID {
				t.Errorf("%s: product %q belongs to workspace %s", want.Slug, p.Name, p.WorkspaceID)
			}
			// A workspace uses one currency, so a request total is
			// meaningful without mixing.
			if p.Currency != want.Products[0].Currency {
				t.Errorf("%s: product %q has currency %q, want %q",
					want.Slug, p.Name, p.Currency, want.Products[0].Currency)
			}
			if p.Stock < 0 {
				t.Errorf("%s: product %q has negative stock", want.Slug, p.Name)
			}
			names[p.Name]++
		}

		for name, count := range names {
			if count > 1 {
				t.Errorf("%s: product %q appears %d times", want.Slug, name, count)
			}
		}
	}

	// The two demo workspaces must not share products.
	if len(results) < 2 {
		t.Fatal("expected at least two demo workspaces")
	}
	foreign, err := q.ListProducts(ctx, sqlc.ListProductsParams{
		WorkspaceID: results[0].ID, PageLimit: 100, PageOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	for _, p := range foreign {
		for _, other := range seed.Demo[1].Products {
			if p.Name == other.Name {
				t.Errorf("product %q leaked into workspace %s", p.Name, results[0].Slug)
			}
		}
	}
}

func TestSeedRefreshRestoresCatalogueValues(t *testing.T) {
	ctx, q := withTx(t)

	results, err := seed.Run(ctx, q, seed.Options{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	ws := results[0]
	target := seed.Demo[0].Products[0]

	product, err := q.GetProductByName(ctx, sqlc.GetProductByNameParams{
		WorkspaceID: ws.ID, Name: target.Name,
	})
	if err != nil {
		t.Fatalf("GetProductByName: %v", err)
	}

	// Simulate an acceptance having consumed stock.
	if _, err := q.DecrementProductStock(ctx, sqlc.DecrementProductStockParams{
		WorkspaceID: ws.ID, ID: product.ID, Quantity: 5,
	}); err != nil {
		t.Fatalf("DecrementProductStock: %v", err)
	}

	// A plain re-run must leave that stock alone, so re-seeding cannot mask
	// the result of a demo acceptance.
	if _, err := seed.Run(ctx, q, seed.Options{}); err != nil {
		t.Fatalf("plain re-run: %v", err)
	}
	after, err := q.GetProduct(ctx, sqlc.GetProductParams{WorkspaceID: ws.ID, ID: product.ID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if after.Stock != target.Stock-5 {
		t.Errorf("plain re-run changed stock to %d, want %d", after.Stock, target.Stock-5)
	}

	// With Refresh it must be restored to the demo baseline.
	refreshed, err := seed.Run(ctx, q, seed.Options{Refresh: true})
	if err != nil {
		t.Fatalf("refresh run: %v", err)
	}
	if refreshed[0].Updated != len(seed.Demo[0].Products) {
		t.Errorf("refresh updated %d products, want %d",
			refreshed[0].Updated, len(seed.Demo[0].Products))
	}
	restored, err := q.GetProduct(ctx, sqlc.GetProductParams{WorkspaceID: ws.ID, ID: product.ID})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if restored.Stock != target.Stock {
		t.Errorf("stock after refresh = %d, want %d", restored.Stock, target.Stock)
	}
}

// The demo data is a contract: the assessment asks for a realistic
// catalogue, and several app behaviours depend on its shape. Pagination is
// only worth demonstrating with more rows than one page holds, and a request
// can only total correctly if the workspace it came from is single currency.
func TestDemoDataShape(t *testing.T) {
	if len(seed.Demo) < 10 {
		t.Fatalf("Demo has %d workspaces, want at least 10", len(seed.Demo))
	}

	// Letters, digits and single spaces. No accents, punctuation or symbols:
	// a name has to survive a URL, a CSV export and a terminal unescaped, and
	// search must behave the same for everyone typing it.
	plain := regexp.MustCompile(`^[A-Za-z0-9]+( [A-Za-z0-9]+)*$`)
	slugShape := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

	slugs := map[string]bool{}
	for _, ws := range seed.Demo {
		if ws.Slug == "" || ws.Name == "" {
			t.Errorf("workspace %+v has an empty name or slug", ws)
		}
		if !plain.MatchString(ws.Name) {
			t.Errorf("workspace name %q is not plain letters, digits and spaces", ws.Name)
		}
		// The database enforces this too; failing here names the culprit.
		if !slugShape.MatchString(ws.Slug) {
			t.Errorf("slug %q does not match the shape the schema requires", ws.Slug)
		}
		if slugs[ws.Slug] {
			t.Errorf("duplicate slug %q", ws.Slug)
		}
		slugs[ws.Slug] = true

		if n := len(ws.Products); n < 30 || n > 50 {
			t.Errorf("%s has %d products, want between 30 and 50", ws.Slug, n)
		}

		names := map[string]bool{}
		currencies := map[string]bool{}
		for _, p := range ws.Products {
			if !plain.MatchString(p.Name) {
				t.Errorf("%s: product name %q is not plain letters, digits and spaces",
					ws.Slug, p.Name)
			}
			// The seed matches products by (workspace, name), so a duplicate
			// name would make the second one silently unreachable.
			if names[p.Name] {
				t.Errorf("%s: duplicate product name %q", ws.Slug, p.Name)
			}
			names[p.Name] = true
			currencies[p.Currency] = true

			if len(p.Currency) != 3 || p.Currency != upper(p.Currency) {
				t.Errorf("%s: product %q has currency %q, want three uppercase letters",
					ws.Slug, p.Name, p.Currency)
			}
			if p.Stock <= 0 {
				t.Errorf("%s: product %q has stock %d, want a positive value",
					ws.Slug, p.Name, p.Stock)
			}
			if _, err := decimalPrice(p.Price); err != nil {
				t.Errorf("%s: product %q has price %q: %v", ws.Slug, p.Name, p.Price, err)
			}
		}

		// A mixed-currency workspace would let a buyer build a basket the
		// service has to reject, which is not what a demo should offer.
		if len(currencies) != 1 {
			t.Errorf("%s spans %d currencies, want exactly 1", ws.Slug, len(currencies))
		}
	}
}

// decimalPrice checks a price is a plain decimal with two places, which is
// what NUMERIC(12,2) stores without rounding.
func decimalPrice(s string) (string, error) {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]{2}$`).MatchString(s) {
		return "", errNotTwoPlaceDecimal
	}
	return s, nil
}

var errNotTwoPlaceDecimal = errors.New("not a decimal with two places")

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}
