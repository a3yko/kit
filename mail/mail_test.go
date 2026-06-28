package mail

import (
	"errors"
	"strings"
	"testing"
)

func TestDisabledWithoutHost(t *testing.T) {
	s := New(Config{})
	if s.Enabled() {
		t.Fatal("Sender with no host should be disabled")
	}
	if err := s.Send("a@b.com", "hi", "body"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Send = %v, want ErrDisabled", err)
	}
}

func TestEnabledWithHost(t *testing.T) {
	if !New(Config{Host: "smtp.example.com"}).Enabled() {
		t.Error("Sender with a host should be enabled")
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(BuildMessage("App <no-reply@x.com>", "u@y.com", "Subj", "line1\nline2"))
	for _, want := range []string{
		"From: App <no-reply@x.com>\r\n",
		"To: u@y.com\r\n",
		"Subject: Subj\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"line1\r\nline2",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "\r\n\r\n") {
		t.Error("expected a blank line between headers and body")
	}
}

func TestSenderAddress(t *testing.T) {
	cases := map[string]string{
		"App <no-reply@x.com>": "no-reply@x.com",
		"plain@x.com":          "plain@x.com",
		"<bare@x.com>":         "bare@x.com",
	}
	for in, want := range cases {
		if got := SenderAddress(in); got != want {
			t.Errorf("SenderAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
