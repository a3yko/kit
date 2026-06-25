package datastarx

// Datastar SSE event names and data-line prefixes.
//
// These track the Datastar protocol version you run — they were renamed across
// releases (the older "merge-fragments" / "merge-signals" became
// "patch-elements" / "patch-signals"). If you upgrade Datastar and the client
// stops reacting, update these constants (or switch to the official SDK).
const (
	EventPatchElements = "datastar-patch-elements"
	EventPatchSignals  = "datastar-patch-signals"
)

// PatchElements streams an HTML fragment for Datastar to patch into the DOM.
// Datastar matches/merges by the element ids in the markup.
func (s *SSE) PatchElements(html string) error {
	return s.Send(EventPatchElements, "elements "+html)
}

// PatchSignals streams a JSON object for Datastar to merge into its signal
// store, e.g. `{"count": 3}`.
func (s *SSE) PatchSignals(jsonObject string) error {
	return s.Send(EventPatchSignals, "signals "+jsonObject)
}
