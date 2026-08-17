package fail_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a3yko/kit/fail"
)

func TestTryPassesValuesThrough(t *testing.T) {
	got := func() (out string, err error) {
		defer fail.Catch(&err)
		return fail.Try("ok", nil), nil
	}
	v, err := got()
	if err != nil || v != "ok" {
		t.Fatalf("got %q, %v", v, err)
	}
}

func TestCatchReturnsTheError(t *testing.T) {
	sentinel := errors.New("boom")
	run := func() (err error) {
		defer fail.Catch(&err)
		_ = fail.Try("", sentinel)
		t.Fatal("Try should not have returned")
		return nil
	}
	if err := run(); !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want %v", err, sentinel)
	}
}

func TestCheckfAddsContext(t *testing.T) {
	run := func() (err error) {
		defer fail.Catch(&err)
		fail.Checkf(io.EOF, "reading %s", "config")
		return nil
	}
	err := run()
	if !errors.Is(err, io.EOF) {
		t.Errorf("lost the wrapped error: %v", err)
	}
	if !strings.Contains(err.Error(), "reading config") {
		t.Errorf("lost the context: %v", err)
	}
}

func TestARealPanicIsNotSwallowed(t *testing.T) {
	// The property that makes this safe. A nil map write is a bug and must keep
	// looking like one; turning it into a returned error is how a crash becomes
	// a silent 500 nobody investigates.
	defer func() {
		if recover() == nil {
			t.Fatal("a genuine panic was converted into an error")
		}
	}()
	func() (err error) {
		defer fail.Catch(&err)
		var m map[string]string
		m["boom"] = "x"
		return nil
	}() //nolint:errcheck
}

func TestHandlerWrites500AndLogsOnce(t *testing.T) {
	var logged strings.Builder
	log := slog.New(slog.NewTextHandler(&logged, nil))
	wrap := fail.Handler(log, nil)

	h := wrap(func(http.ResponseWriter, *http.Request) error {
		_ = fail.Try("", errors.New("database is on fire"))
		return nil
	})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/stock", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(logged.String(), "database is on fire") {
		t.Errorf("error not logged: %s", logged.String())
	}
	if strings.Count(logged.String(), "database is on fire") != 1 {
		t.Errorf("logged more than once: %s", logged.String())
	}
}

func TestHandlerCanRespondCustomly(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	wrap := fail.Handler(log, func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := wrap(func(http.ResponseWriter, *http.Request) error {
		return errors.New("plain returned error, no panic")
	})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", rec.Code)
	}
}

func TestHandlerLeavesSuccessAlone(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := fail.Handler(log, nil)(func(w http.ResponseWriter, _ *http.Request) error {
		_, _ = w.Write([]byte("fine"))
		return nil
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "fine" {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}
