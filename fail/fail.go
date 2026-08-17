// Package fail removes the `if err != nil` noise from linear code, without
// removing the error handling.
//
// # What this is
//
// Go has no `?` operator, and it is not getting one. What it does have is
// panic/recover, and the standard library already uses exactly this pattern
// internally where a deeply recursive routine would otherwise thread an error
// through every frame - encoding/json and text/template both do it. The rule
// that makes it safe rather than reckless is that the panic must never cross a
// package boundary: it is raised by Try and caught by Catch in the same call
// stack, and anything else is a bug.
//
// # Before
//
//	func (s *Server) handleStock(w http.ResponseWriter, r *http.Request) {
//	    products, err := s.catalog.ListProducts(ctx, tid, q, "")
//	    if err != nil {
//	        s.fail(w, "list products", err)
//	        return
//	    }
//	    summary, err := s.catalog.Summary(ctx, tid)
//	    if err != nil {
//	        s.fail(w, "summary", err)
//	        return
//	    }
//	    defs, err := s.catalog.Fields(ctx, tid)
//	    if err != nil {
//	        s.fail(w, "fields", err)
//	        return
//	    }
//	    ...
//	}
//
// # After
//
//	func (s *Server) handleStock(w http.ResponseWriter, r *http.Request) (err error) {
//	    defer fail.Catch(&err)
//	    products := fail.Try(s.catalog.ListProducts(ctx, tid, q, ""))
//	    summary  := fail.Try(s.catalog.Summary(ctx, tid))
//	    defs     := fail.Try(s.catalog.Fields(ctx, tid))
//	    ...
//	}
//
// Twelve lines become three, and the error still propagates - it is returned,
// not swallowed. The handler's signature changes to return an error, which
// Handler adapts back to an http.HandlerFunc.
//
// # When NOT to use it
//
//   - When the error is not fatal to the operation. `if errors.Is(err, ErrNoRows)`
//     followed by a default is a decision, not noise, and Try would turn it into
//     a 500. Handle those explicitly, as normal.
//   - Across a package boundary. Never let a Try panic escape a Catch.
//   - In a library others call. Exported functions should return errors.
//
// The honest summary: this is good for the "do six things in a row, any of which
// could fail, all of which end the request" shape - which is most HTTP handlers -
// and wrong everywhere else. It is a scalpel, not a policy.
package fail

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// wrapped is the panic payload. A distinct type is what lets Catch tell "an
// error we raised" apart from "a nil map write", and re-panic the second kind
// rather than quietly converting a real bug into a 500.
type wrapped struct{ err error }

// Try returns v, or panics if err is non-nil. Recovered by Catch.
func Try[T any](v T, err error) T {
	if err != nil {
		panic(wrapped{err})
	}
	return v
}

// Try2 is Try for a function returning two values and an error.
func Try2[A, B any](a A, b B, err error) (A, B) {
	if err != nil {
		panic(wrapped{err})
	}
	return a, b
}

// Check panics if err is non-nil. For calls that return only an error.
func Check(err error) {
	if err != nil {
		panic(wrapped{err})
	}
}

// Checkf is Check with context added to the error.
func Checkf(err error, format string, args ...any) {
	if err != nil {
		panic(wrapped{fmt.Errorf(format+": %w", append(args, err)...)})
	}
}

// Catch recovers a Try panic into *dst. Use it deferred, with a named return:
//
//	func doWork() (err error) {
//	    defer fail.Catch(&err)
//	    ...
//	}
//
// A panic that did not come from this package is re-raised unchanged. A nil map
// write is a bug and must keep looking like one - converting it into a returned
// error is how a crash becomes a silent 500 that nobody investigates.
func Catch(dst *error) {
	r := recover()
	if r == nil {
		return
	}
	w, ok := r.(wrapped)
	if !ok {
		panic(r)
	}
	*dst = w.err
}

// HandlerFunc is an http.HandlerFunc that may return an error.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// Handler adapts a fallible handler to net/http, catching Try panics, logging
// once, and writing a 500 - so a handler never repeats that block.
//
// respond lets the caller decide what a failure looks like: a plain 500 for an
// HTML page, a Datastar toast for a fragment request. Pass nil for a plain 500.
func Handler(log *slog.Logger, respond func(http.ResponseWriter, *http.Request, error)) func(HandlerFunc) http.HandlerFunc {
	return func(h HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			err := invoke(h, w, r)
			if err == nil {
				return
			}
			log.Error("handler failed",
				"path", r.URL.Path, "method", r.Method, "err", err)
			if respond != nil {
				respond(w, r, err)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}

// invoke runs h with a Catch in place. Separate from Handler so the deferred
// recover has its own frame and cannot swallow a panic raised by the logging.
func invoke(h HandlerFunc, w http.ResponseWriter, r *http.Request) (err error) {
	defer Catch(&err)
	return h(w, r)
}

// Recovered reports whether err came out of a Catch rather than a return. Rarely
// needed; useful when a caller wants to log a stack for the panic path only.
func Recovered(err error) bool {
	var w wrapped
	return errors.As(err, &w)
}

func (w wrapped) Error() string { return w.err.Error() }
func (w wrapped) Unwrap() error { return w.err }

// Stack returns the current stack, for logging alongside a recovered panic.
func Stack() string { return string(debug.Stack()) }
