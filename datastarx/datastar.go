package datastarx

import (
	"io"
	"strings"
)

// Datastar SSE event names.
//
// These track the Datastar protocol version you run — they were renamed across
// releases (the older "merge-fragments" / "merge-signals" became
// "patch-elements" / "patch-signals"). If you upgrade Datastar and the client
// stops reacting, update these constants (or switch to the official SDK).
const (
	EventPatchElements = "datastar-patch-elements"
	EventPatchSignals  = "datastar-patch-signals"
)

// PatchElements streams an HTML fragment for Datastar to patch into the DOM
// (matched/merged by element id). Multi-line HTML is handled correctly.
func (s *SSE) PatchElements(html string) error {
	return s.patch(EventPatchElements, "elements", html)
}

// PatchSignals streams a JSON object for Datastar to merge into its signal
// store, e.g. `{"count": 3}`.
func (s *SSE) PatchSignals(jsonObject string) error {
	return s.patch(EventPatchSignals, "signals", jsonObject)
}

// patch writes one Datastar event whose value may span multiple lines. Datastar
// requires the data key (e.g. "elements") on every "data:" line, so we prefix
// each line — not just the first.
func (s *SSE) patch(event, key, value string) error {
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(event)
	b.WriteByte('\n')
	for line := range strings.SplitSeq(value, "\n") { // Go 1.24 iterator: no slice alloc
		b.WriteString("data: ")
		b.WriteString(key)
		b.WriteByte(' ')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	if _, err := io.WriteString(s.w, b.String()); err != nil {
		return err
	}
	return s.rc.Flush()
}
