# tenancy

Resolve the current tenant from an HTTP request — **custom domain first, then
subdomain** — and carry it on the request context.

The resolution logic and middleware live here; **you supply the lookups**
(`Lookup`) that map a host or subdomain to your tenant id.

## Install

```sh
go get github.com/a3yko/kit/tenancy
```

## What you implement

```go
type Lookup interface {
    BySubdomain(ctx context.Context, sub string) (tenantID string, err error)
    ByCustomDomain(ctx context.Context, host string) (tenantID string, err error)
}
```

Return `tenancy.ErrNoTenant` (or any error) when there's no match.

## Usage

```go
resolver := tenancy.NewResolver("app.com", lookup) // "app.com" is your apex

// Middleware resolves the tenant onto the context, or 404s on no match.
// Pass a custom http.HandlerFunc for the not-found case, or nil for http.NotFound.
handler := resolver.Middleware(nil)(appHandler)

// Inside a handler:
tenantID, ok := tenancy.TenantID(r.Context())
```

Resolution order for a request to `host`:

1. `ByCustomDomain(host)` — e.g. `cars.acme.com`
2. else `BySubdomain(sub)` where `sub` is the left label under your apex — e.g.
   `acme` from `acme.app.com`

Ports are stripped automatically. The apex, `www`, and multi-label hosts are
**not** treated as subdomains.

## Resolve without middleware

```go
id, err := resolver.Resolve(ctx, r.Host)
if errors.Is(err, tenancy.ErrNoTenant) { /* unknown host */ }
```

## Helpers

```go
sub, ok := tenancy.Subdomain("acme.app.com", "app.com") // "acme", true
```

## Note

Go has no ORM-style default scope, so **per-query tenant scoping stays your
responsibility** — read `tenancy.TenantID(ctx)` and filter every query by it.
This package gets the id onto the context reliably; it can't enforce that you
use it.
