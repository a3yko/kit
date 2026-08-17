// Package enum makes a closed set of string values unforgeable.
//
// Go has no sum types. The usual stand-in is a named string:
//
//	type Status string
//	const Active Status = "active"
//
// which documents intent and enforces nothing. `Status("shipped")` compiles.
// `Status(row.Status)` compiles, and quietly admits whatever the database
// happens to hold - including a value written by a migration you have not read,
// or a typo in a seed script. The compiler sees a string conversion and is
// satisfied. In an inventory or ledger system that is the beginning of a very
// bad afternoon.
//
// This package borrows two ideas from ML-family languages:
//
//	abstract types  a value whose representation is hidden, so the only way to
//	                obtain one is through functions the owning module provides
//	phantom types   a type parameter that carries no data and exists purely to
//	                stop two otherwise-identical types being interchangeable
//
// Both are expressible in Go. Value's field is unexported, so no package outside
// this one can construct one by conversion. The Tag type parameter is never
// stored, so Value[orderStatus] and Value[productStatus] are different types
// that cannot be passed to each other's functions - which a plain `string` or
// even a named string type cannot give you.
//
// # Usage
//
//	// The tag is unexported: only this package can name the type.
//	type statusTag struct{}
//
//	// Status is now an abstract type. There is no literal that produces one.
//	type Status = enum.Value[statusTag]
//
//	var Statuses = enum.New[statusTag]("stub", "draft", "active")
//
//	var (
//	    Stub   = Statuses.MustParse("stub")   // panics at init if misspelled
//	    Draft  = Statuses.MustParse("draft")
//	    Active = Statuses.MustParse("active")
//	)
//
// At every boundary where an untrusted string arrives - a database column, a URL
// parameter, a JSON body - you must parse:
//
//	status, err := Statuses.Parse(row.Status)
//
// which is the point. "Parse, don't validate": once past that line the value is
// known good by construction, and every function downstream can rely on it
// without re-checking.
//
// # What this does not do
//
// It does not give you exhaustive matching. Go cannot check that a switch covers
// every case, and no library can add that. What it does give you is All(), so a
// test can assert that whatever table you switch on covers the set - see
// Set.All and the exhaustiveness pattern in the README. A test is weaker than a
// compiler, and it is a great deal better than nothing.
package enum

import (
	"fmt"
	"slices"
	"strings"
)

// Value is one member of a closed set.
//
// The zero Value is deliberately not a member of any set: an uninitialised
// field is detectably invalid rather than silently meaning the first constant.
// Check with IsZero.
type Value[Tag any] struct {
	s string
}

// String returns the underlying text, for rendering and for storage.
func (v Value[Tag]) String() string { return v.s }

// IsZero reports whether this is the zero Value - never a legitimate member.
func (v Value[Tag]) IsZero() bool { return v.s == "" }

// MarshalText makes Value work with encoding/json and friends.
func (v Value[Tag]) MarshalText() ([]byte, error) { return []byte(v.s), nil }

// Set is a closed collection of Values sharing a Tag.
type Set[Tag any] struct {
	ordered []Value[Tag]
	index   map[string]Value[Tag]
}

// New builds a set. Order is preserved, because it is usually meaningful - a
// lifecycle runs in an order, and a dropdown should render in it.
//
// Panics on a duplicate or an empty member: both are programming errors, and
// this runs at package initialisation where a panic is a failed build rather
// than a failed request.
func New[Tag any](members ...string) *Set[Tag] {
	s := &Set[Tag]{index: make(map[string]Value[Tag], len(members))}
	for _, m := range members {
		if m == "" {
			panic("enum: empty member")
		}
		if _, dup := s.index[m]; dup {
			panic("enum: duplicate member " + m)
		}
		v := Value[Tag]{s: m}
		s.index[m] = v
		s.ordered = append(s.ordered, v)
	}
	return s
}

// Parse converts an untrusted string into a Value, or reports why it cannot.
// This is the only way to get a Value from data.
func (s *Set[Tag]) Parse(raw string) (Value[Tag], error) {
	v, ok := s.index[raw]
	if !ok {
		return Value[Tag]{}, fmt.Errorf("enum: %q is not one of: %s", raw, strings.Join(s.Names(), ", "))
	}
	return v, nil
}

// MustParse is Parse for values that are known good at compile time - the
// package-level constants of the owning module. Panics otherwise, at init.
func (s *Set[Tag]) MustParse(raw string) Value[Tag] {
	v, err := s.Parse(raw)
	if err != nil {
		panic(err)
	}
	return v
}

// Contains reports whether raw names a member.
func (s *Set[Tag]) Contains(raw string) bool {
	_, ok := s.index[raw]
	return ok
}

// All returns every member in declaration order. Use it to drive a UI, and to
// drive the exhaustiveness test that stands in for a compiler check.
func (s *Set[Tag]) All() []Value[Tag] { return slices.Clone(s.ordered) }

// Names returns every member as a string, for building a CHECK constraint or an
// error message.
func (s *Set[Tag]) Names() []string {
	out := make([]string, 0, len(s.ordered))
	for _, v := range s.ordered {
		out = append(out, v.s)
	}
	return out
}

// SQLList renders the members as a SQL IN list: 'a', 'b', 'c'.
//
// So the CHECK constraint in a migration and the Go set can be checked against
// each other by a test, rather than drifting apart the first time somebody adds
// a member to one and not the other.
func (s *Set[Tag]) SQLList() string {
	quoted := make([]string, 0, len(s.ordered))
	for _, v := range s.ordered {
		quoted = append(quoted, "'"+v.s+"'")
	}
	return strings.Join(quoted, ", ")
}

// Len returns the number of members.
func (s *Set[Tag]) Len() int { return len(s.ordered) }
