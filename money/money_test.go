package money

import (
	"errors"
	"testing"
)

func TestCents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1234.56", 123456},
		{"0", 0},
		{"-5", -500},
		{"10.5", 1050},
		{"1,000", 100000},
		{"  42  ", 4200},
		{".50", 50},
		{"0.999", 100}, // round half-up on third decimal
		{"1.005", 101}, // ditto
		{"1.004", 100}, // truncate when < 5
		{"-0.01", -1},
		{"+7", 700},
	}
	for _, c := range cases {
		got, err := Cents(c.in)
		if err != nil {
			t.Errorf("Cents(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Cents(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCentsInvalid(t *testing.T) {
	for _, in := range []string{"", "abc", "1.2.3", "$5", "."} {
		if _, err := Cents(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Cents(%q) err = %v, want ErrInvalid", in, err)
		}
	}
}

func TestDecimal(t *testing.T) {
	cases := map[int64]string{123456: "1234.56", -50000: "-500.00", 0: "0.00", 5: "0.05"}
	for in, want := range cases {
		if got := Decimal(in); got != want {
			t.Errorf("Decimal(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		cents int64
		cur   string
		want  string
	}{
		{123456, "USD", "$1,234.56"},
		{-200000, "EUR", "-€2,000.00"},
		{4900, "GBP", "£49.00"},
		{490000, "PLN", "4,900.00 PLN"},
		{5, "", "$0.05"},
		{1000000, "USD", "$10,000.00"},
	}
	for _, c := range cases {
		if got := Format(c.cents, c.cur); got != c.want {
			t.Errorf("Format(%d,%q) = %q, want %q", c.cents, c.cur, got, c.want)
		}
	}
}

// Round-trip: a value formatted as a bare decimal parses back to the same cents.
func TestDecimalRoundTrip(t *testing.T) {
	for _, cents := range []int64{0, 1, 99, 100, 123456, -50000, 999999} {
		got, err := Cents(Decimal(cents))
		if err != nil || got != cents {
			t.Errorf("round-trip %d -> %q -> %d (err %v)", cents, Decimal(cents), got, err)
		}
	}
}
