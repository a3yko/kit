package opt_test

import (
	"testing"

	"github.com/a3yko/kit/opt"
)

func TestZeroValueIsNone(t *testing.T) {
	// So a struct that gains an Optional field does not silently start claiming
	// it has data.
	var o opt.Optional[string]
	if o.Present() {
		t.Error("the zero Optional claims to be present")
	}
	if _, ok := o.Get(); ok {
		t.Error("Get returned ok on a zero Optional")
	}
}

func TestSomeAndNone(t *testing.T) {
	v, ok := opt.Some("316L").Get()
	if !ok || v != "316L" {
		t.Errorf("Some: got %q, %v", v, ok)
	}
	if _, ok := opt.None[string]().Get(); ok {
		t.Error("None reported present")
	}
}

func TestFromPtrIsTheDatabaseBoundary(t *testing.T) {
	// sqlc emits *T for a nullable column; this is where NULL becomes absence.
	if opt.FromPtr[string](nil).Present() {
		t.Error("nil became present")
	}
	s := "widget"
	if v, _ := opt.FromPtr(&s).Get(); v != "widget" {
		t.Errorf("got %q", v)
	}
}

func TestPtrRoundTrips(t *testing.T) {
	s := "widget"
	back := opt.FromPtr(&s).Ptr()
	if back == nil || *back != "widget" {
		t.Fatalf("lost the value: %v", back)
	}
	if opt.None[string]().Ptr() != nil {
		t.Error("None produced a non-nil pointer")
	}
}

func TestPtrDoesNotAliasTheOriginal(t *testing.T) {
	// Optional holds a copy; handing out a pointer to its interior would let a
	// caller mutate a value that is supposed to be settled.
	s := "before"
	o := opt.FromPtr(&s)
	s = "after"
	if v, _ := o.Get(); v != "before" {
		t.Errorf("Optional aliased its source: %q", v)
	}
}

func TestOrAndOrZero(t *testing.T) {
	if got := opt.None[int]().Or(42); got != 42 {
		t.Errorf("Or: got %d", got)
	}
	if got := opt.Some(7).Or(42); got != 7 {
		t.Errorf("Or: got %d", got)
	}
	if got := opt.None[string]().OrZero(); got != "" {
		t.Errorf("OrZero: got %q", got)
	}
}

func TestMap(t *testing.T) {
	got := opt.Map(opt.Some(3), func(n int) string { return string(rune('a' + n)) })
	if v, ok := got.Get(); !ok || v != "d" {
		t.Errorf("got %q %v", v, ok)
	}
	if opt.Map(opt.None[int](), func(int) string { return "x" }).Present() {
		t.Error("Map produced a value from None")
	}
}
