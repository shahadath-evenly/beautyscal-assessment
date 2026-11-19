// Package money converts the database's exact decimal type to and from the
// string form the API uses.
//
// Money never passes through a float. NUMERIC(12,2) is exact, float64 is not:
// 0.1 + 0.2 is not 0.3 in binary floating point, and a cent lost that way is
// invisible until it is a reconciliation problem. JSON numbers are doubles in
// most clients, so amounts cross the wire as strings.
package money

import (
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// Scale is the number of decimal places the schema stores.
const Scale = 2

// String renders a NUMERIC as a fixed-point decimal string, always with two
// decimal places: "49.90", not "49.9".
func String(n pgtype.Numeric) (string, error) {
	if !n.Valid {
		return "", fmt.Errorf("money: value is null")
	}
	if n.NaN {
		return "", fmt.Errorf("money: value is NaN")
	}
	if n.InfinityModifier != pgtype.Finite {
		return "", fmt.Errorf("money: value is not finite")
	}

	// pgtype stores the value as Int * 10^Exp. Rescaling to Scale decimal
	// places keeps the arithmetic in big.Int, never in floating point.
	value := new(big.Int).Set(n.Int)
	exp := int(n.Exp) + Scale

	switch {
	case exp > 0:
		value.Mul(value, pow10(exp))
	case exp < 0:
		// Values are stored at scale 2, so this only trims trailing zeros.
		value.Quo(value, pow10(-exp))
	}

	negative := value.Sign() < 0
	digits := new(big.Int).Abs(value).String()
	for len(digits) <= Scale {
		digits = "0" + digits
	}

	split := len(digits) - Scale
	out := digits[:split] + "." + digits[split:]
	if negative {
		out = "-" + out
	}
	return out, nil
}

// MustString is String for values known to be valid, such as a column read
// from a NOT NULL field. It returns "0.00" rather than failing a response for
// a value the schema should have made impossible.
func MustString(n pgtype.Numeric) string {
	s, err := String(n)
	if err != nil {
		return "0.00"
	}
	return s
}

// Parse converts a decimal string into the exact NUMERIC the columns store.
func Parse(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("money: %q is not a valid decimal: %w", s, err)
	}
	return n, nil
}

// MultiplyByQuantity returns amount * quantity as a decimal string, used for
// line and order totals. The multiplication stays in big.Int.
func MultiplyByQuantity(amount pgtype.Numeric, quantity int32) (string, error) {
	if !amount.Valid {
		return "", fmt.Errorf("money: amount is null")
	}
	product := pgtype.Numeric{
		Int:   new(big.Int).Mul(amount.Int, big.NewInt(int64(quantity))),
		Exp:   amount.Exp,
		Valid: true,
	}
	return String(product)
}

// Sum adds decimal strings of the same currency, returning a decimal string.
// Callers must only sum amounts in one currency; unlike currencies are never
// added.
func Sum(amounts ...string) (string, error) {
	total := big.NewInt(0)
	for _, a := range amounts {
		n, err := Parse(a)
		if err != nil {
			return "", err
		}
		scaled := new(big.Int).Set(n.Int)
		exp := int(n.Exp) + Scale
		switch {
		case exp > 0:
			scaled.Mul(scaled, pow10(exp))
		case exp < 0:
			scaled.Quo(scaled, pow10(-exp))
		}
		total.Add(total, scaled)
	}
	return String(pgtype.Numeric{Int: total, Exp: -Scale, Valid: true})
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
