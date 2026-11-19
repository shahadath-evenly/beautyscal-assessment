package money_test

import (
	"testing"

	"github.com/real-musafir/b-scale/backend/internal/platform/money"
)

func TestStringKeepsTwoDecimalPlaces(t *testing.T) {
	cases := map[string]string{
		"49.90":    "49.90",
		"49.9":     "49.90",
		"0":        "0.00",
		"0.05":     "0.05",
		"19.5":     "19.50",
		"1234.56":  "1234.56",
		"68":       "68.00",
		"99999.99": "99999.99",
	}
	for in, want := range cases {
		n, err := money.Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		got, err := money.String(n)
		if err != nil {
			t.Fatalf("String(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestMultiplyByQuantityIsExact(t *testing.T) {
	cases := []struct {
		price    string
		quantity int32
		want     string
	}{
		{"49.90", 2, "99.80"},
		{"0.10", 3, "0.30"}, // the classic float trap: 0.1*3 != 0.30000000000000004
		{"19.99", 7, "139.93"},
		{"0.01", 100, "1.00"},
		{"68.00", 0, "0.00"},
	}
	for _, tc := range cases {
		price, err := money.Parse(tc.price)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.price, err)
		}
		got, err := money.MultiplyByQuantity(price, tc.quantity)
		if err != nil {
			t.Fatalf("MultiplyByQuantity: %v", err)
		}
		if got != tc.want {
			t.Errorf("%s x %d = %q, want %q", tc.price, tc.quantity, got, tc.want)
		}
	}
}

func TestSumIsExact(t *testing.T) {
	got, err := money.Sum("0.10", "0.20")
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if got != "0.30" {
		t.Errorf("0.10 + 0.20 = %q, want 0.30", got)
	}

	total, err := money.Sum("99.80", "19.50", "0.05")
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if total != "119.35" {
		t.Errorf("total = %q, want 119.35", total)
	}

	empty, err := money.Sum()
	if err != nil {
		t.Fatalf("Sum(): %v", err)
	}
	if empty != "0.00" {
		t.Errorf("empty sum = %q, want 0.00", empty)
	}
}

func TestParseRejectsNonsense(t *testing.T) {
	for _, in := range []string{"abc", "", "12.3.4"} {
		if _, err := money.Parse(in); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", in)
		}
	}
}
