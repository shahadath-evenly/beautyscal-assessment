package orderrequest

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/money"
)

func product(t *testing.T, id uuid.UUID, name, price, currency string) sqlc.Product {
	t.Helper()
	amount, err := money.Parse(price)
	if err != nil {
		t.Fatalf("parse %q: %v", price, err)
	}
	return sqlc.Product{ID: id, Name: name, Price: amount, Currency: currency, Stock: 100}
}

func TestAggregateQuantitiesMergesRepeatedProducts(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	got := aggregateQuantities([]CreateItemInput{
		{ProductID: a, Quantity: 2},
		{ProductID: b, Quantity: 1},
		{ProductID: a, Quantity: 3},
	})

	if len(got) != 2 {
		t.Fatalf("aggregated into %d lines, want 2", len(got))
	}

	totals := map[uuid.UUID]int32{}
	for _, item := range got {
		totals[item.ProductID] = item.Quantity
	}
	if totals[a] != 5 {
		t.Errorf("quantity for the repeated product = %d, want 5", totals[a])
	}
	if totals[b] != 1 {
		t.Errorf("quantity for the single product = %d, want 1", totals[b])
	}

	// Deterministic order, matching the lock order the accept flow will use.
	for i := 1; i < len(got); i++ {
		if got[i-1].ProductID.String() > got[i].ProductID.String() {
			t.Error("aggregated items are not sorted by product ID")
		}
	}
}

func TestBuildLinesTakesPricesFromTheDatabase(t *testing.T) {
	id := uuid.New()
	products := []sqlc.Product{product(t, id, "Rose Serum", "49.90", "EUR")}

	lines, err := buildLines([]CreateItemInput{{ProductID: id, Quantity: 2}}, products)
	if err != nil {
		t.Fatalf("buildLines: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("built %d lines, want 1", len(lines))
	}

	line := lines[0]
	// The name, price and currency are snapshots of the catalogue row, not
	// anything the client supplied.
	if line.ProductName != "Rose Serum" || line.Currency != "EUR" || line.Quantity != 2 {
		t.Errorf("unexpected line %+v", line)
	}
	if got := money.MustString(line.UnitPrice); got != "49.90" {
		t.Errorf("unit price = %q, want 49.90", got)
	}
}

func TestBuildLinesRejectsUnknownProducts(t *testing.T) {
	known, unknown := uuid.New(), uuid.New()
	products := []sqlc.Product{product(t, known, "Known", "10.00", "EUR")}

	_, err := buildLines([]CreateItemInput{
		{ProductID: known, Quantity: 1},
		{ProductID: unknown, Quantity: 1},
	}, products)

	var unknownErr *UnknownProductsError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("error = %v, want *UnknownProductsError", err)
	}
	if len(unknownErr.IDs) != 1 || unknownErr.IDs[0] != unknown {
		t.Errorf("reported %v, want just %s", unknownErr.IDs, unknown)
	}
	if len(unknownErr.Details()) != 1 {
		t.Errorf("details = %+v, want one entry", unknownErr.Details())
	}
}

func TestBuildLinesRejectsMixedCurrencies(t *testing.T) {
	eur, usd := uuid.New(), uuid.New()
	products := []sqlc.Product{
		product(t, eur, "Euro Serum", "10.00", "EUR"),
		product(t, usd, "Dollar Mask", "20.00", "USD"),
	}

	_, err := buildLines([]CreateItemInput{
		{ProductID: eur, Quantity: 1},
		{ProductID: usd, Quantity: 1},
	}, products)

	var mixed *MixedCurrencyError
	if !errors.As(err, &mixed) {
		t.Fatalf("error = %v, want *MixedCurrencyError", err)
	}
	if len(mixed.Currencies) != 2 {
		t.Errorf("currencies = %v, want both", mixed.Currencies)
	}
}

func TestTotalsAreExact(t *testing.T) {
	price, err := money.Parse("49.90")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cheap, err := money.Parse("0.10")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	total, currency, err := totals([]sqlc.OrderRequestItem{
		{ProductID: uuid.New(), Quantity: 2, UnitPrice: price, Currency: "EUR"},
		{ProductID: uuid.New(), Quantity: 3, UnitPrice: cheap, Currency: "EUR"},
	})
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	// 2 x 49.90 + 3 x 0.10 = 99.80 + 0.30
	if total != "100.10" {
		t.Errorf("total = %q, want 100.10", total)
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want EUR", currency)
	}
}

func TestTotalsOfNoItems(t *testing.T) {
	total, currency, err := totals(nil)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if total != "0.00" || currency != "" {
		t.Errorf("total = %q, currency = %q; want 0.00 and empty", total, currency)
	}
}

func TestIsValidStatus(t *testing.T) {
	for _, valid := range []string{"new", "accepted", "rejected"} {
		if !IsValidStatus(valid) {
			t.Errorf("%q should be valid", valid)
		}
	}
	for _, invalid := range []string{"NEW", "cancelled", "", "draft"} {
		if IsValidStatus(invalid) {
			t.Errorf("%q should be invalid", invalid)
		}
	}
}
