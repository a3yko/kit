package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContextEnrichment(t *testing.T) {
	var buf bytes.Buffer
	SetJSON(&buf, slog.LevelInfo)

	ctx := WithRequestID(context.Background(), "req-1")
	ctx = WithUserID(ctx, "user-7")
	ctx = WithTenantID(ctx, "tenant-9")
	Info(ctx, "hello", "extra", "v")

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("log line not JSON: %v (%q)", err, buf.String())
	}
	for k, want := range map[string]string{
		"request_id": "req-1", "user_id": "user-7", "tenant_id": "tenant-9",
		"extra": "v", "msg": "hello",
	} {
		if got, _ := rec[k].(string); got != want {
			t.Errorf("field %q = %q, want %q", k, got, want)
		}
	}
}

func TestMiddlewareStampsRequestIDAndLogs(t *testing.T) {
	var buf bytes.Buffer
	SetJSON(&buf, slog.LevelInfo)

	var seenID string
	h := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestID(r.Context())
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/vehicles", nil))

	if seenID == "" {
		t.Fatal("handler saw empty request id on context")
	}
	if got := rec.Header().Get(HeaderRequestID); got != seenID {
		t.Errorf("response header %q = %q, want %q", HeaderRequestID, got, seenID)
	}

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("request log not JSON: %v (%q)", err, buf.String())
	}
	if line["request_id"] != seenID {
		t.Errorf("logged request_id = %v, want %q", line["request_id"], seenID)
	}
	if line["status"].(float64) != http.StatusTeapot {
		t.Errorf("logged status = %v, want %d", line["status"], http.StatusTeapot)
	}
	if line["bytes"].(float64) != 2 {
		t.Errorf("logged bytes = %v, want 2", line["bytes"])
	}
}

func TestMiddlewareTrustsInboundID(t *testing.T) {
	SetJSON(new(bytes.Buffer), slog.LevelInfo)
	var seen string
	h := Middleware(TrustInboundID())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "upstream-42")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "upstream-42" {
		t.Errorf("request id = %q, want inbound %q", seen, "upstream-42")
	}
}

func TestMiddlewareSkipsLogButStillStamps(t *testing.T) {
	var buf bytes.Buffer
	SetJSON(&buf, slog.LevelInfo)
	var seen string
	h := Requests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/up", nil))
	if seen == "" {
		t.Error("skipped path should still get a request id on context")
	}
	if strings.Contains(buf.String(), "http request") {
		t.Errorf("expected no request log for /up, got %q", buf.String())
	}
}

func TestSSEFlusherPreserved(t *testing.T) {
	// The wrapped writer must still satisfy http.Flusher or Datastar SSE breaks.
	h := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("wrapped ResponseWriter no longer implements http.Flusher")
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
