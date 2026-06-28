// Package dbx holds the small, repetitive helpers you reach for when building
// sqlc/pgx query params from ordinary Go values: wrapping a uuid/time in the
// pgtype the generated code wants, and converting between "" / zero and a
// nullable pointer. They're one-liners — the point is to write them once instead
// of in every domain package.
package dbx

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUID wraps a uuid.UUID as a valid pgtype.UUID.
func UUID(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }

// NullUUID wraps u as a valid pgtype.UUID, or an invalid (SQL NULL) one when u is
// the zero uuid.
func NullUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: u != uuid.Nil}
}

// PtrUUID wraps an optional uuid pointer (nil -> SQL NULL).
func PtrUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

// Date wraps a time as a valid pgtype.Date.
func Date(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

// Timestamptz wraps a time as a valid pgtype.Timestamptz.
func Timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Ptr returns a pointer to v, or nil when v is the zero value — for nullable
// columns where empty means NULL. Generic over comparable types ("" -> nil, 0 ->
// nil, etc.).
func Ptr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

// Deref returns *p, or the zero value of T when p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns *p, or def when p is nil.
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}
