// Package tenancy resolves the current tenant from an HTTP request — by custom
// domain first, then by subdomain — and carries it on the request context.
//
// The resolution logic and middleware live here; your app supplies the lookups
// ([Lookup]) that map a host/subdomain to your tenant id.
package tenancy

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// ErrNoTenant means the request host matched no tenant.
var ErrNoTenant = errors.New("tenancy: no tenant for host")

// Subdomain returns the left-most label of host under root — e.g.
// host="acme.app.com", root="app.com" yields ("acme", true). It returns
// ok=false when host is not a strict single-label subdomain of root (that
// excludes the apex, "www", and multi-label hosts). Any port in host is ignored.
func Subdomain(host, root string) (string, bool) {
	host = stripPort(host)
	if host == "" || root == "" {
		return "", false
	}
	suffix := "." + root
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || sub == "www" || strings.Contains(sub, ".") {
		return "", false
	}
	return sub, true
}

func stripPort(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}

// Lookup maps request hosts to your tenant id. Return ErrNoTenant (or any error)
// when there is no match; a returned error is treated as "no match".
type Lookup interface {
	BySubdomain(ctx context.Context, sub string) (tenantID string, err error)
	ByCustomDomain(ctx context.Context, host string) (tenantID string, err error)
}

// Resolver resolves a tenant id from a request host.
type Resolver struct {
	root   string
	lookup Lookup
}

// NewResolver builds a resolver. root is your apex domain (e.g. "app.com"),
// used for subdomain matching.
func NewResolver(root string, lookup Lookup) *Resolver {
	return &Resolver{root: root, lookup: lookup}
}

// Resolve returns the tenant id for host. Custom domain takes precedence over
// subdomain; it returns ErrNoTenant if neither matches.
func (r *Resolver) Resolve(ctx context.Context, host string) (string, error) {
	host = stripPort(host)
	if id, err := r.lookup.ByCustomDomain(ctx, host); err == nil && id != "" {
		return id, nil
	}
	if sub, ok := Subdomain(host, r.root); ok {
		if id, err := r.lookup.BySubdomain(ctx, sub); err == nil && id != "" {
			return id, nil
		}
	}
	return "", ErrNoTenant
}

// Middleware resolves the tenant and stores it on the request context. When no
// tenant matches it calls notFound (or [http.NotFound] if nil) and stops.
func (r *Resolver) Middleware(notFound http.HandlerFunc) func(http.Handler) http.Handler {
	if notFound == nil {
		notFound = http.NotFound
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, err := r.Resolve(req.Context(), req.Host)
			if err != nil {
				notFound(w, req)
				return
			}
			next.ServeHTTP(w, req.WithContext(WithTenantID(req.Context(), id)))
		})
	}
}
