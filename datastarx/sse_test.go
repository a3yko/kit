package datastarx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatchSignals(t *testing.T) {
	rec := httptest.NewRecorder()
	sse, err := NewSSE(rec)
	if err != nil {
		t.Fatalf("NewSSE: %v", err)
	}
	if err := sse.PatchSignals(`{"count":3}`); err != nil {
		t.Fatalf("PatchSignals: %v", err)
	}

	want := "event: datastar-patch-signals\ndata: signals {\"count\":3}\n\n"
	if got := rec.Body.String(); !strings.Contains(got, want) {
		t.Errorf("body = %q, want it to contain %q", got, want)
	}
}

func TestPatchElementsKeysEveryLine(t *testing.T) {
	rec := httptest.NewRecorder()
	sse, err := NewSSE(rec)
	if err != nil {
		t.Fatalf("NewSSE: %v", err)
	}
	if err := sse.PatchElements("<div>\n  <span>hi</span>\n</div>"); err != nil {
		t.Fatalf("PatchElements: %v", err)
	}

	// Datastar needs the "elements" key on every data line, not just the first.
	want := "event: datastar-patch-elements\n" +
		"data: elements <div>\n" +
		"data: elements   <span>hi</span>\n" +
		"data: elements </div>\n\n"
	if got := rec.Body.String(); !strings.Contains(got, want) {
		t.Errorf("body = %q\nwant contains %q", got, want)
	}
}

func TestSendSplitsMultilineData(t *testing.T) {
	rec := httptest.NewRecorder()
	sse, err := NewSSE(rec)
	if err != nil {
		t.Fatalf("NewSSE: %v", err)
	}
	if err := sse.Send("e", "line1\nline2"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := "event: e\ndata: line1\ndata: line2\n\n"
	if got := rec.Body.String(); !strings.Contains(got, want) {
		t.Errorf("body = %q, want it to contain %q", got, want)
	}
}
