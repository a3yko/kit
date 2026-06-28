// Package money formats and parses amounts held as integer minor units (cents).
// Integer cents is the only safe in-memory representation for money; this package
// is the one place to turn a user's "1234.56" into 123456 and back, so rounding
// and formatting are consistent everywhere.
package money

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid is returned by Cents for an unparseable amount.
var ErrInvalid = errors.New("money: invalid amount")

// Cents parses a decimal money string into integer cents WITHOUT float rounding,
// e.g. "1234.56" -> 123456, "-5" -> -500, "10.5" -> 1050, "1,000" -> 100000. It
// accepts an optional sign, thousands separators, and up to two fractional digits
// (a third or more rounds half-up).
func Cents(s string) (int64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, ErrInvalid
	}
	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}

	intPart, fracPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	if intPart == "" && fracPart == "" {
		return 0, ErrInvalid
	}

	var whole int64
	for i := 0; i < len(intPart); i++ {
		if intPart[i] < '0' || intPart[i] > '9' {
			return 0, ErrInvalid
		}
		whole = whole*10 + int64(intPart[i]-'0')
	}
	for i := 0; i < len(fracPart); i++ {
		if fracPart[i] < '0' || fracPart[i] > '9' {
			return 0, ErrInvalid
		}
	}

	var cents int64
	switch {
	case len(fracPart) == 0:
		cents = 0
	case len(fracPart) == 1:
		cents = int64(fracPart[0]-'0') * 10
	default:
		cents = int64(fracPart[0]-'0')*10 + int64(fracPart[1]-'0')
		if len(fracPart) >= 3 && fracPart[2] >= '5' {
			cents++ // round half-up on the third decimal
		}
	}

	total := whole*100 + cents
	if neg {
		total = -total
	}
	return total, nil
}

// Decimal renders integer cents as a bare two-place decimal suitable for an HTML
// number input's value, e.g. 123456 -> "1234.56", -50000 -> "-500.00" (no symbol,
// no separators).
func Decimal(cents int64) string {
	sign, c := splitSign(cents)
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

// Format renders integer cents for display with a currency symbol and thousands
// separators, e.g. (123456,"USD") -> "$1,234.56", (-200000,"EUR") -> "-€2,000.00".
// Currencies without a known symbol get a trailing code: (490000,"PLN") ->
// "4,900.00 PLN". An empty currency is treated as USD.
func Format(cents int64, currency string) string {
	sign, c := splitSign(cents)
	body := groupThousands(c/100) + fmt.Sprintf(".%02d", c%100)
	switch strings.ToUpper(currency) {
	case "USD", "":
		return sign + "$" + body
	case "EUR":
		return sign + "€" + body
	case "GBP":
		return sign + "£" + body
	default:
		return sign + body + " " + strings.ToUpper(currency)
	}
}

func splitSign(cents int64) (string, int64) {
	if cents < 0 {
		return "-", -cents
	}
	return "", cents
}

// groupThousands inserts comma separators into a non-negative integer.
func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}
