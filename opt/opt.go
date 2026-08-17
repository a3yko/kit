// Package opt is OCaml's `'a option` in Go.
//
// # Why
//
// Go's usual answer to "this value may be absent" is a pointer, or a pair of
// fields:
//
//	Name    string
//	HasName bool
//
// Both are wrong in the same way: nothing stops the two halves drifting apart.
// Set Name without setting HasName and the value is invisible; set HasName
// without setting Name and you render an empty cell as though it were data. The
// compiler helps with neither, and the bug is silent.
//
// A pointer has the same problem in a different costume: `*string` says "maybe
// absent" and also "maybe shared" and also "maybe nil because someone forgot",
// and dereferencing it is a nil panic rather than a compile error.
//
//	Optional[string]
//
// says exactly one thing, and the only way to read it is Get, which forces you
// to acknowledge absence at the point of use. That is the whole idea behind
// OCaml's option type, and it survives the trip to Go intact.
//
// # The zero value is None, deliberately
//
// A zero-valued Optional is absent, not "present and empty". So a struct that
// gains an Optional field does not silently start claiming it has data.
package opt

// Optional holds a value that may be absent.
//
// The fields are unexported: outside this package the only way to build one is
// Some or None, so a half-initialised Optional cannot exist. That is the same
// guarantee an abstract type in an OCaml module signature gives you.
type Optional[T any] struct {
	value   T
	present bool
}

// Some wraps a present value.
func Some[T any](v T) Optional[T] { return Optional[T]{value: v, present: true} }

// None is an absent value. The zero Optional is already None; this is for
// saying so out loud at a call site.
func None[T any]() Optional[T] { return Optional[T]{} }

// Get returns the value and whether it was present. This is the only accessor
// that cannot lie, and the comma-ok shape makes ignoring absence deliberate
// rather than accidental.
func (o Optional[T]) Get() (T, bool) { return o.value, o.present }

// Present reports whether a value is set.
func (o Optional[T]) Present() bool { return o.present }

// OrZero returns the value, or T's zero value when absent. Use it only where the
// zero is genuinely a sensible stand-in - an empty string in a template cell,
// not a zero price in an invoice.
func (o Optional[T]) OrZero() T {
	var zero T
	if !o.present {
		return zero
	}
	return o.value
}

// Or returns the value, or fallback when absent.
func (o Optional[T]) Or(fallback T) T {
	if !o.present {
		return fallback
	}
	return o.value
}

// FromPtr converts a possibly-nil pointer, which is what sqlc emits for a
// nullable column. This is the boundary where a database NULL becomes an option.
func FromPtr[T any](p *T) Optional[T] {
	if p == nil {
		return None[T]()
	}
	return Some(*p)
}

// Ptr converts back, for handing a value to a query that expects a nullable.
func (o Optional[T]) Ptr() *T {
	if !o.present {
		return nil
	}
	v := o.value
	return &v
}

// Map applies f to a present value. A free function rather than a method,
// because Go methods cannot introduce a new type parameter.
func Map[A, B any](o Optional[A], f func(A) B) Optional[B] {
	v, ok := o.Get()
	if !ok {
		return None[B]()
	}
	return Some(f(v))
}
