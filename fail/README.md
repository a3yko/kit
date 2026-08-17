# fail

Removes the `if err != nil` noise from linear code, without removing the error
handling.

## Why

Go has no `?` operator and is not getting one. It does have panic/recover, and
the standard library already uses exactly this pattern internally where threading
an error through every frame would drown the logic — `encoding/json` and
`text/template` both do it.

The rule that makes it safe rather than reckless: **the panic never crosses a
package boundary.** It is raised by `Try` and caught by `Catch` in the same call
stack, and it carries a private type so anything else is re-raised untouched.

## Install

```sh
go get github.com/a3yko/kit/fail
```

## Before

```go
func (s *Server) handleStock(w http.ResponseWriter, r *http.Request) {
    products, err := s.catalog.ListProducts(ctx, tid, q, "")
    if err != nil {
        s.fail(w, "list products", err)
        return
    }
    summary, err := s.catalog.Summary(ctx, tid)
    if err != nil {
        s.fail(w, "summary", err)
        return
    }
    defs, err := s.catalog.Fields(ctx, tid)
    if err != nil {
        s.fail(w, "fields", err)
        return
    }
    ...
}
```

## After

```go
func (s *Server) handleStock(w http.ResponseWriter, r *http.Request) (err error) {
    defer fail.Catch(&err)
    products := fail.Try(s.catalog.ListProducts(ctx, tid, q, ""))
    summary  := fail.Try(s.catalog.Summary(ctx, tid))
    defs     := fail.Try(s.catalog.Fields(ctx, tid))
    ...
}
```

Twelve lines become three, and the error still propagates — it is returned, not
swallowed.

## Wiring it to net/http

```go
h := fail.Handler(logger, respond)   // respond may be nil for a plain 500

r.Get("/stock", h(s.handleStock))
```

`Handler` catches, logs once, and lets you decide what a failure looks like — a
plain 500 for a page, a toast for a fragment request.

## When NOT to use it

- When the error is not fatal to the operation. `if errors.Is(err, ErrNoRows)`
  followed by a default is a *decision*, not noise, and `Try` would turn it into
  a 500.
- Across a package boundary. Never let a `Try` panic escape a `Catch`.
- In a library others call. Exported functions return errors.

A genuine panic — a nil map write — is re-raised, not converted. A crash must
keep looking like a crash; turning it into a returned error is how a bug becomes
a silent 500 nobody investigates. That is tested.

## Scope

Good for "do six things in a row, any of which ends the request" — which is most
HTTP handlers. Wrong everywhere else. A scalpel, not a policy.
