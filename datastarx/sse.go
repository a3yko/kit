// Package datastarx contains small helpers for driving [Datastar] responses
// over Server-Sent Events from net/http.
//
// It is intentionally minimal: a correct SSE writer ([SSE]) plus thin helpers
// for Datastar's patch-elements / patch-signals events. It is not a full wrapper
// of the Datastar protocol — for that, use the official Datastar Go SDK. The
// event names here track the Datastar version you run (see datastar.go).
//
// [Datastar]: https://data-star.dev
package datastarx

import (
	"fmt"
	"net/http"
	"strings"
)

// SSE is a minimal Server-Sent Events writer over an http.ResponseWriter.
type SSE struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSE sets the SSE response headers and returns a writer. It errors if the
// ResponseWriter does not support flushing, which streaming requires.
func NewSSE(w http.ResponseWriter) (*SSE, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("datastarx: ResponseWriter does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &SSE{w: w, flusher: flusher}, nil
}

// Send writes one SSE event with the given event name and data lines, then
// flushes. Each string in data may contain newlines; every line is emitted as
// its own "data:" field, per the SSE spec.
func (s *SSE) Send(event string, data ...string) error {
	var b strings.Builder
	if event != "" {
		fmt.Fprintf(&b, "event: %s\n", event)
	}
	for _, d := range data {
		for _, line := range strings.Split(d, "\n") {
			fmt.Fprintf(&b, "data: %s\n", line)
		}
	}
	b.WriteByte('\n')

	if _, err := s.w.Write([]byte(b.String())); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
