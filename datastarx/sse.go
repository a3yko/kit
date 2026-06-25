// Package datastarx contains small helpers for driving [Datastar] responses
// over Server-Sent Events from net/http.
//
// It is intentionally minimal: a correct SSE writer ([SSE]) plus thin helpers
// for Datastar's patch-elements / patch-signals events. It is not a full wrapper
// of the Datastar protocol — for that, use the official Datastar Go SDK. The
// event names here track the Datastar version you run (see datastar.go).
//
// An SSE is bound to one request/response and is not safe for concurrent use.
//
// [Datastar]: https://data-star.dev
package datastarx

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SSE is a minimal Server-Sent Events writer over an http.ResponseWriter.
type SSE struct {
	w  io.Writer
	rc *http.ResponseController
}

// NewSSE sets the SSE response headers and returns a writer. It errors if the
// response cannot be flushed (streaming requires it). It uses
// http.ResponseController, so it still works when middleware wraps the
// ResponseWriter, unlike a bare http.Flusher type assertion.
func NewSSE(w http.ResponseWriter) (*SSE, error) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // tell nginx & friends not to buffer the stream

	rc := http.NewResponseController(w)
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return nil, fmt.Errorf("datastarx: response is not flushable: %w", err)
	}
	return &SSE{w: w, rc: rc}, nil
}

// Send writes one SSE event with the given event name and data lines, then
// flushes. Each string in data may contain newlines; every line is emitted as
// its own "data:" field, per the SSE spec.
func (s *SSE) Send(event string, data ...string) error {
	var b strings.Builder
	if event != "" {
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteByte('\n')
	}
	for _, d := range data {
		for line := range strings.SplitSeq(d, "\n") { // Go 1.24 iterator: no slice alloc
			b.WriteString("data: ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')

	if _, err := io.WriteString(s.w, b.String()); err != nil {
		return err
	}
	return s.rc.Flush()
}
