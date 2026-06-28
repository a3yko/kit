package log

import (
	"net/http"
	"strings"
	"time"
)

// HeaderRequestID is the request/response header carrying the request id.
const HeaderRequestID = "X-Request-ID"

// responseWriter captures the status code and byte count while passing through
// the http.Flusher interface that Server-Sent-Events (e.g. Datastar) rely on.
type responseWriter struct {
	http.ResponseWriter
	status  int
	bytes   int
	written bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.status = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.status = http.StatusOK
		rw.written = true
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Flush forwards to the underlying writer so SSE handlers can stream.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer (Go 1.20+).
func (rw *responseWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }

type config struct {
	header       string
	trustInbound bool
	skipPaths    map[string]struct{}
	skipPrefixes []string
	genID        func() string
}

// Option configures the logging middleware.
type Option func(*config)

// WithHeader sets the request-id header name (default X-Request-ID).
func WithHeader(name string) Option { return func(c *config) { c.header = name } }

// TrustInboundID reuses a client-supplied request-id header instead of always
// generating one. Enable only behind a trusted proxy that sets it.
func TrustInboundID() Option { return func(c *config) { c.trustInbound = true } }

// SkipPaths suppresses the per-request log line for exact path matches
// (health checks, etc.). The request id is still stamped onto the context.
func SkipPaths(paths ...string) Option {
	return func(c *config) {
		for _, p := range paths {
			c.skipPaths[p] = struct{}{}
		}
	}
}

// SkipPrefixes suppresses the per-request log line for path prefixes (e.g.
// "/static/").
func SkipPrefixes(prefixes ...string) Option {
	return func(c *config) { c.skipPrefixes = append(c.skipPrefixes, prefixes...) }
}

// WithIDGenerator overrides how a new request id is produced (default
// NewRequestID).
func WithIDGenerator(fn func() string) Option { return func(c *config) { c.genID = fn } }

// Middleware returns net/http middleware that assigns each request a request id
// (echoed in the response header and put on the context for log.From/log.Info),
// then logs one structured line per request with method, path, status, bytes and
// duration. User and tenant ids are picked up automatically if a downstream
// handler stamps them via WithUserID / WithTenantID before the response starts —
// but those typically appear on lines the handler logs itself.
func Middleware(opts ...Option) func(http.Handler) http.Handler {
	c := &config{header: HeaderRequestID, skipPaths: map[string]struct{}{}, genID: NewRequestID}
	for _, o := range opts {
		o(c)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			id := ""
			if c.trustInbound {
				id = r.Header.Get(c.header)
			}
			if id == "" {
				id = c.genID()
			}
			ctx := WithRequestID(r.Context(), id)
			w.Header().Set(c.header, id)

			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r.WithContext(ctx))

			if c.skip(r.URL.Path) {
				return
			}
			LogRequest(ctx, r.Method, r.URL.Path, rw.status, rw.bytes, time.Since(start))
		})
	}
}

// Requests is Middleware with defaults plus the usual noise filters: it skips
// logging /static/ assets, common health endpoints and favicon/service-worker.
func Requests(next http.Handler) http.Handler {
	return Middleware(
		SkipPrefixes("/static/"),
		SkipPaths("/up", "/health", "/healthz", "/favicon.ico", "/sw.js"),
	)(next)
}

func (c *config) skip(path string) bool {
	if _, ok := c.skipPaths[path]; ok {
		return true
	}
	for _, p := range c.skipPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
