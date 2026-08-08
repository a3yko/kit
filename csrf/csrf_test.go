package csrf

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// req builds a request addressed to host, with optional Origin/Referer headers.
func req(method, host, path string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, "http://"+host+path, nil)
	r.Host = host
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestVerifyAllows(t *testing.T) {
	const host = "app.example.com"
	p := New()

	cases := []struct {
		name    string
		method  string
		headers map[string]string
	}{
		// Safe methods are never gated, whatever the origin claims.
		{"GET with a hostile origin", "GET", map[string]string{"Origin": "https://evil.test"}},
		{"HEAD with a hostile origin", "HEAD", map[string]string{"Origin": "https://evil.test"}},
		{"OPTIONS preflight", "OPTIONS", map[string]string{"Origin": "https://evil.test"}},

		// The ordinary same-origin form post.
		{"matching origin", "POST", map[string]string{"Origin": "https://app.example.com"}},
		{"matching origin over http", "POST", map[string]string{"Origin": "http://app.example.com"}},
		{"origin case is ignored", "POST", map[string]string{"Origin": "https://APP.Example.COM"}},
		{"explicit default port", "POST", map[string]string{"Origin": "http://app.example.com:80"}},

		// Referer is the fallback when Origin is absent.
		{"matching referer", "POST", map[string]string{"Referer": "http://app.example.com/vehicles/1"}},

		// Neither header: not a browser. Webhooks authenticate by signature.
		{"no browser headers", "POST", nil},
		{"empty header values", "POST", map[string]string{"Origin": "", "Referer": ""}},

		// Other unsafe methods behave the same.
		{"same-origin DELETE", "DELETE", map[string]string{"Origin": "http://app.example.com"}},
		{"same-origin PATCH", "PATCH", map[string]string{"Origin": "http://app.example.com"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := p.Verify(req(tc.method, host, "/x", tc.headers)); err != nil {
				t.Errorf("expected the request to pass, got %v", err)
			}
		})
	}
}

func TestVerifyRejects(t *testing.T) {
	const host = "app.example.com"
	p := New()

	cases := []struct {
		name    string
		method  string
		headers map[string]string
	}{
		{"cross-origin POST", "POST", map[string]string{"Origin": "https://evil.test"}},
		{"cross-origin PUT", "PUT", map[string]string{"Origin": "https://evil.test"}},
		{"cross-origin DELETE", "DELETE", map[string]string{"Origin": "https://evil.test"}},
		{"cross-origin PATCH", "PATCH", map[string]string{"Origin": "https://evil.test"}},

		// A sibling subdomain is a different origin — this is the multi-tenant
		// case the check exists to catch.
		{"sibling subdomain", "POST", map[string]string{"Origin": "https://other.example.com"}},
		{"parent domain", "POST", map[string]string{"Origin": "https://example.com"}},

		// Suffix and prefix tricks against a naive string comparison.
		{"host as a suffix of the attacker's", "POST", map[string]string{"Origin": "https://app.example.com.evil.test"}},
		{"host embedded in a path", "POST", map[string]string{"Origin": "https://evil.test/app.example.com"}},
		{"userinfo trick", "POST", map[string]string{"Origin": "https://app.example.com@evil.test"}},

		{"port mismatch", "POST", map[string]string{"Origin": "http://app.example.com:9090"}},
		{"opaque null origin", "POST", map[string]string{"Origin": "null"}},
		{"unparseable origin", "POST", map[string]string{"Origin": "not a url"}},

		{"mismatched referer", "POST", map[string]string{"Referer": "https://evil.test/x"}},
		// Origin wins when both are present, so a good Referer cannot launder a bad Origin.
		{"bad origin beats good referer", "POST", map[string]string{
			"Origin": "https://evil.test", "Referer": "http://app.example.com/x",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := p.Verify(req(tc.method, host, "/x", tc.headers))
			if err == nil {
				t.Fatal("expected the request to be rejected")
			}
			if !errors.Is(err, ErrCrossOrigin) {
				t.Errorf("expected ErrCrossOrigin, got %v", err)
			}
		})
	}
}

func TestVerifyNoHost(t *testing.T) {
	r := req("POST", "app.example.com", "/x", map[string]string{"Origin": "https://app.example.com"})
	r.Host = ""
	r.URL.Host = ""

	err := New().Verify(r)
	if !errors.Is(err, ErrNoHost) {
		t.Fatalf("expected ErrNoHost, got %v", err)
	}
}

// A TLS request must compare against the https default port, so that an Origin
// with no port and a Host carrying :443 both normalise to the bare hostname.
func TestVerifyTLSDefaultPort(t *testing.T) {
	p := New()

	r := req("POST", "app.example.com:443", "/x", map[string]string{"Origin": "https://app.example.com"})
	r.TLS = &tls.ConnectionState{}
	if err := p.Verify(r); err != nil {
		t.Errorf("https + :443 should match a portless origin, got %v", err)
	}

	// Without TLS the scheme is http, so :443 is not a default and must not be dropped.
	r2 := req("POST", "app.example.com:443", "/x", map[string]string{"Origin": "https://app.example.com"})
	if err := p.Verify(r2); err == nil {
		t.Error("http + :443 should not match a portless https origin")
	}
}

func TestTrustForwardedHost(t *testing.T) {
	headers := map[string]string{
		"Origin":            "https://public.example.com",
		"X-Forwarded-Host":  "public.example.com",
		"X-Forwarded-Proto": "https",
	}

	// The proxy rewrote Host to an internal name, so the default configuration
	// sees a mismatch...
	if err := New().Verify(req("POST", "internal.local:8090", "/x", headers)); err == nil {
		t.Error("without TrustForwardedHost the internal host should not match")
	}
	// ...and opting in compares against the public hostname instead.
	if err := New(TrustForwardedHost()).Verify(req("POST", "internal.local:8090", "/x", headers)); err != nil {
		t.Errorf("with TrustForwardedHost the public host should match, got %v", err)
	}

	// Several proxies append to the header; the client-most value is the first.
	chained := map[string]string{
		"Origin":            "https://public.example.com",
		"X-Forwarded-Host":  "public.example.com, inner.local",
		"X-Forwarded-Proto": "https, http",
	}
	if err := New(TrustForwardedHost()).Verify(req("POST", "internal.local", "/x", chained)); err != nil {
		t.Errorf("the first forwarded value should be used, got %v", err)
	}
}

func TestAllowOrigins(t *testing.T) {
	const host = "app.example.com"

	p := New(AllowOrigins("https://shell.example.net"))
	if err := p.Verify(req("POST", host, "/x", map[string]string{"Origin": "https://shell.example.net"})); err != nil {
		t.Errorf("an allowed origin should pass, got %v", err)
	}
	if err := p.Verify(req("POST", host, "/x", map[string]string{"Origin": "https://evil.test"})); err == nil {
		t.Error("allowing one origin must not allow every origin")
	}
	// null stays rejected unless it is allowed explicitly.
	if err := p.Verify(req("POST", host, "/x", map[string]string{"Origin": "null"})); err == nil {
		t.Error("null should still be rejected")
	}
	if err := New(AllowOrigins("null")).Verify(req("POST", host, "/x", map[string]string{"Origin": "null"})); err != nil {
		t.Errorf("explicitly allowed null should pass, got %v", err)
	}
	// An allowed origin is matched on scheme+host+port, not by substring.
	if err := p.Verify(req("POST", host, "/x", map[string]string{"Origin": "https://shell.example.net.evil.test"})); err == nil {
		t.Error("an allowed origin must not match by suffix")
	}
}

func TestSkip(t *testing.T) {
	const host = "app.example.com"
	hostile := map[string]string{"Origin": "https://evil.test"}

	p := New(SkipPaths("/webhooks/sumup"), SkipPrefixes("/api/"))
	if err := p.Verify(req("POST", host, "/webhooks/sumup", hostile)); err != nil {
		t.Errorf("a skipped path should pass, got %v", err)
	}
	if err := p.Verify(req("POST", host, "/api/v1/things", hostile)); err != nil {
		t.Errorf("a skipped prefix should pass, got %v", err)
	}
	if err := p.Verify(req("POST", host, "/webhooks/other", hostile)); err == nil {
		t.Error("an unskipped path should still be checked")
	}
}

func TestMiddleware(t *testing.T) {
	const host = "app.example.com"
	var reached bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	// Rejected: 403, and the handler never runs.
	reached = false
	rec := httptest.NewRecorder()
	Protect(next).ServeHTTP(rec, req("POST", host, "/x", map[string]string{"Origin": "https://evil.test"}))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
	if reached {
		t.Error("the handler must not run for a rejected request")
	}

	// Allowed: passes through.
	reached = false
	rec = httptest.NewRecorder()
	Protect(next).ServeHTTP(rec, req("POST", host, "/x", map[string]string{"Origin": "http://app.example.com"}))
	if rec.Code != http.StatusOK || !reached {
		t.Errorf("expected the handler to run and return 200, got %d reached=%v", rec.Code, reached)
	}

	// The rejection response says nothing about why.
	rec = httptest.NewRecorder()
	Protect(next).ServeHTTP(rec, req("POST", host, "/x", map[string]string{"Origin": "https://evil.test"}))
	if strings.Contains(rec.Body.String(), "evil.test") {
		t.Errorf("the rejection body should not echo the origin: %q", rec.Body.String())
	}
}

func TestOnReject(t *testing.T) {
	rec := httptest.NewRecorder()
	custom := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	})
	handler := New(OnReject(custom)).Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler must not run for a rejected request")
	}))

	handler.ServeHTTP(rec, req("POST", "app.example.com", "/x", map[string]string{"Origin": "https://evil.test"}))
	if rec.Code != http.StatusTeapot {
		t.Errorf("expected the custom status 418, got %d", rec.Code)
	}
}
