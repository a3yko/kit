// Package log is structured request/job logging on top of log/slog: a process
// logger plus context propagation so every line a request (or background job)
// emits carries the same request_id, user_id, tenant_id and job_id without
// threading them through every call.
//
// Set the process logger once at startup, then log against the context:
//
//	log.SetJSON(os.Stdout, slog.LevelInfo)
//	// ... in a handler, after log.Middleware has run:
//	log.Info(ctx, "vehicle created", "vehicle_id", id)
//	// => {"level":"INFO","msg":"vehicle created","request_id":"a1b2c3d4","vehicle_id":"..."}
//
// The HTTP middleware (Middleware / Requests) stamps a request id onto the
// context and logs one line per request; SetJobID does the same for workers.
package log

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
	tenantIDKey  contextKey = "tenant_id"
	jobIDKey     contextKey = "job_id"
)

// current holds the process logger. atomic so SetDefault is race-free if called
// after serving has begun, though the intended use is once at startup.
var current atomic.Pointer[slog.Logger]

func init() {
	// Default to JSON at info on stdout so a binary logs sensibly with no setup.
	SetJSON(os.Stdout, slog.LevelInfo)
}

// SetDefault installs l as the process logger returned by Default / From.
func SetDefault(l *slog.Logger) { current.Store(l) }

// SetJSON installs a JSON-handler logger at the given level (production default).
func SetJSON(w io.Writer, level slog.Level) {
	SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})))
}

// SetText installs a text-handler logger at the given level (handy in dev).
func SetText(w io.Writer, level slog.Level) {
	SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}

// Default returns the process logger without any context enrichment.
func Default() *slog.Logger { return current.Load() }

// From returns the process logger with whatever identifiers are present on ctx
// (request_id, user_id, tenant_id, job_id) attached as attributes.
func From(ctx context.Context) *slog.Logger {
	l := current.Load()
	if ctx == nil {
		return l
	}
	if v := RequestID(ctx); v != "" {
		l = l.With("request_id", v)
	}
	if v := UserID(ctx); v != "" {
		l = l.With("user_id", v)
	}
	if v := TenantID(ctx); v != "" {
		l = l.With("tenant_id", v)
	}
	if v := JobID(ctx); v != "" {
		l = l.With("job_id", v)
	}
	return l
}

// Context setters — each returns a derived context carrying the identifier.

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}
func WithJobID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, jobIDKey, id)
}

// Context getters — each returns "" when the identifier is absent.

func RequestID(ctx context.Context) string { return strVal(ctx, requestIDKey) }
func UserID(ctx context.Context) string    { return strVal(ctx, userIDKey) }
func TenantID(ctx context.Context) string  { return strVal(ctx, tenantIDKey) }
func JobID(ctx context.Context) string     { return strVal(ctx, jobIDKey) }

func strVal(ctx context.Context, k contextKey) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(k).(string); ok {
		return v
	}
	return ""
}

// Context-aware level helpers.

func Debug(ctx context.Context, msg string, args ...any) { From(ctx).Debug(msg, args...) }
func Info(ctx context.Context, msg string, args ...any)  { From(ctx).Info(msg, args...) }
func Warn(ctx context.Context, msg string, args ...any)  { From(ctx).Warn(msg, args...) }
func Error(ctx context.Context, msg string, args ...any) { From(ctx).Error(msg, args...) }

// NewRequestID returns a short, random, URL-safe hex id (8 bytes) for tagging a
// request or job when the caller hasn't supplied one.
func NewRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// rand.Read effectively never fails; fall back to a time-derived id.
		return hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}

// LogRequest emits one structured line for a finished HTTP request, choosing the
// level from the status code (>=500 error, >=400 warn, else info).
func LogRequest(ctx context.Context, method, path string, status int, bytes int, d time.Duration) {
	level := slog.LevelInfo
	switch {
	case status >= 500:
		level = slog.LevelError
	case status >= 400:
		level = slog.LevelWarn
	}
	From(ctx).LogAttrs(ctx, level, "http request",
		slog.String("method", method),
		slog.String("path", path),
		slog.Int("status", status),
		slog.Int("bytes", bytes),
		slog.Int64("duration_ms", d.Milliseconds()),
	)
}

// LogJobResult emits one structured line for a finished background job.
func LogJobResult(ctx context.Context, jobType string, start time.Time, err error) {
	d := time.Since(start)
	if err != nil {
		From(ctx).Error("job failed",
			"job_type", jobType, "duration_ms", d.Milliseconds(), "error", err.Error())
		return
	}
	From(ctx).Info("job completed", "job_type", jobType, "duration_ms", d.Milliseconds())
}
