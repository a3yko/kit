// Package csrf protects state-changing requests by checking that they were sent
// from the same origin they are addressed to — no hidden form tokens, no session
// storage, no template changes.
//
// The rule: a POST/PUT/PATCH/DELETE carrying a browser Origin (or, absent that,
// a Referer) must have been sent to that same host. A request carrying neither
// header is not a browser request and is allowed through, so signature-verified
// server-to-server callers — Stripe and SumUp webhooks, health probes, curl —
// keep working untouched.
//
// This is the strategy Django applies to HTTPS requests, and it composes with a
// SameSite=Lax session cookie rather than replacing it:
//
//   - SameSite=Lax stops the browser attaching the session to a cross-site POST;
//   - this check rejects anything that slips past that — an older browser, a
//     same-site-but-different-host attacker, a redirect-laundered form.
//
// Comparing against the request's own Host, rather than a configured origin, is
// what makes it work for multi-tenant apps: workspace subdomains and customer
// custom domains are self-consistent with no allowlist to maintain.
//
// Wire it in ahead of your routes, outside the session middleware:
//
//	mux = csrf.Protect(mux)
//
// Behind a reverse proxy that rewrites the Host, use TrustForwardedHost so the
// check compares against the public hostname the browser actually used:
//
//	mux = csrf.New(csrf.TrustForwardedHost()).Middleware(mux)
package csrf

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrCrossOrigin is returned by Verify when a state-changing request's Origin or
// Referer names a different host than the one it was sent to.
var ErrCrossOrigin = errors.New("csrf: cross-origin request")

// ErrNoHost is returned by Verify when a browser-originated state-changing
// request arrives with no Host header. HTTP/1.1 requires one; without it there
// is nothing to compare against, so the request is refused rather than guessed at.
var ErrNoHost = errors.New("csrf: request has no host")

type config struct {
	trustForwarded bool
	allowed        map[string]struct{}
	skipPaths      map[string]struct{}
	skipPrefixes   []string
	onReject       http.Handler
}

// Option configures a Protector.
type Option func(*config)

// TrustForwardedHost makes the check read the public host and scheme from
// X-Forwarded-Host / X-Forwarded-Proto instead of r.Host and r.TLS. Enable it
// only behind a proxy that sets those headers and strips client-supplied copies
// — otherwise a caller can forge the value it is checked against and defeat the
// protection entirely.
//
// You do not need this for the common nginx setup that forwards the original
// Host (`proxy_set_header Host $host`), because r.Host already holds it.
func TrustForwardedHost() Option {
	return func(c *config) { c.trustForwarded = true }
}

// AllowOrigins permits specific extra origins that are not the request's own
// host, compared scheme+host+port after normalising default ports. Use it for a
// separate front end or a native WebView shell that posts from a custom scheme;
// pass "null" to admit sandboxed-iframe and some file:// WebView contexts, which
// are otherwise rejected.
//
// Every entry widens the attack surface — a compromised allowed origin can drive
// authenticated writes. Prefer serving the front end from the same origin.
func AllowOrigins(origins ...string) Option {
	return func(c *config) {
		for _, o := range origins {
			o = strings.ToLower(strings.TrimSpace(o))
			if o == "" {
				continue
			}
			if o == "null" {
				c.allowed["null"] = struct{}{}
				continue
			}
			if h := originOf(o); h != "" {
				c.allowed[h] = struct{}{}
			}
		}
	}
}

// SkipPaths exempts exact paths from the check. Reach for it only when a caller
// legitimately sends a browser Origin that will not match — a webhook that sends
// no Origin is already allowed and needs no exemption.
func SkipPaths(paths ...string) Option {
	return func(c *config) {
		for _, p := range paths {
			c.skipPaths[p] = struct{}{}
		}
	}
}

// SkipPrefixes exempts path prefixes from the check. See SkipPaths.
func SkipPrefixes(prefixes ...string) Option {
	return func(c *config) { c.skipPrefixes = append(c.skipPrefixes, prefixes...) }
}

// OnReject replaces the response written for a rejected request (by default a
// plain 403). The Verify error is not passed to it: a rejected request should
// learn nothing about why.
func OnReject(h http.Handler) Option {
	return func(c *config) { c.onReject = h }
}

// Protector applies the same-origin check. The zero value is not usable; build
// one with New.
type Protector struct{ c config }

// New builds a Protector from the given options.
func New(opts ...Option) *Protector {
	c := config{
		allowed:   map[string]struct{}{},
		skipPaths: map[string]struct{}{},
		onReject: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		}),
	}
	for _, o := range opts {
		o(&c)
	}
	return &Protector{c: c}
}

// Verify reports why a request must be rejected, or nil if it may proceed. The
// returned error wraps ErrCrossOrigin or ErrNoHost and carries the offending
// values, so it is safe to log but not to show a caller.
//
// Use it directly when a handler needs to decide for itself; most callers want
// Middleware instead.
func (p *Protector) Verify(r *http.Request) error {
	if safeMethod(r.Method) || p.skip(r.URL.Path) {
		return nil
	}

	// Only the first present of Origin/Referer is consulted. A browser always
	// sends Origin on a cross-origin write, so Origin-absent-but-Referer-present
	// is the older same-origin case, and neither present is a non-browser client.
	source, header := r.Header.Get("Origin"), "origin"
	if source == "" {
		source, header = r.Header.Get("Referer"), "referer"
	}
	if source == "" {
		return nil
	}

	// An explicitly allowed origin bypasses the host comparison. "null" is not a
	// URL, so it is matched literally.
	if strings.EqualFold(strings.TrimSpace(source), "null") {
		if _, ok := p.c.allowed["null"]; ok {
			return nil
		}
		return fmt.Errorf("%w: opaque %s", ErrCrossOrigin, header)
	}
	origin := originOf(source)
	if origin == "" {
		return fmt.Errorf("%w: unparseable %s %q", ErrCrossOrigin, header, source)
	}
	if _, ok := p.c.allowed[origin]; ok {
		return nil
	}

	host := p.host(r)
	if host == "" {
		return ErrNoHost
	}
	if origin != host {
		return fmt.Errorf("%w: %s %q does not match host %q", ErrCrossOrigin, header, origin, host)
	}
	return nil
}

// Middleware returns net/http middleware that runs Verify and rejects failures
// before the request reaches next.
func (p *Protector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := p.Verify(r); err != nil {
			p.c.onReject.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Protect is Middleware with default options — the one-liner for the common case.
func Protect(next http.Handler) http.Handler { return New().Middleware(next) }

// host returns the normalised "host[:port]" the request was addressed to, with
// the port dropped when it is the default for the scheme (so it lines up with
// what a browser puts in Origin).
func (p *Protector) host(r *http.Request) string {
	host, scheme := r.Host, "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p.c.trustForwarded {
		if h := firstValue(r.Header.Get("X-Forwarded-Host")); h != "" {
			host = h
		}
		if s := firstValue(r.Header.Get("X-Forwarded-Proto")); s != "" {
			scheme = strings.ToLower(s)
		}
	}
	// Parsing as a URL gives correct IPv6 bracket handling for free.
	u, err := url.Parse(scheme + "://" + strings.TrimSpace(host))
	if err != nil {
		return ""
	}
	return joinHostPort(strings.ToLower(u.Hostname()), u.Port(), scheme)
}

// originOf reduces an absolute URL to its normalised "host[:port]", or "" if the
// value is not an absolute URL with a host.
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return joinHostPort(strings.ToLower(u.Hostname()), u.Port(), strings.ToLower(u.Scheme))
}

// joinHostPort drops the port when it is the scheme's default, so that
// "https://x.test" and "x.test:443" compare equal.
func joinHostPort(host, port, scheme string) string {
	if host == "" {
		return ""
	}
	if port == "" || (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		return host
	}
	return host + ":" + port
}

// firstValue takes the first entry of a comma-separated proxy header, which
// holds the client-most value when several proxies have appended to it.
func firstValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// safeMethod reports whether the method is read-only per RFC 9110 and so cannot
// be used to change state.
func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	}
	return false
}

func (p *Protector) skip(path string) bool {
	if _, ok := p.c.skipPaths[path]; ok {
		return true
	}
	for _, prefix := range p.c.skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
