// Package mail sends transactional email over SMTP. It is intentionally tiny: a
// single Send for a plain-text message. When SMTP is not configured the Sender
// is disabled and Send returns ErrDisabled, so callers can degrade gracefully
// (e.g. show a copyable link instead of emailing it).
package mail

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"
)

// ErrDisabled is returned by Send when no SMTP host is configured.
var ErrDisabled = errors.New("mail: SMTP not configured")

// Config is the SMTP relay configuration. An empty Host disables sending.
type Config struct {
	Host string
	Port int    // default 587 when zero
	User string // when non-empty, PLAIN auth is used
	Pass string
	From string // RFC 5322 From, e.g. "App <no-reply@example.com>"
}

// Sender sends mail via an SMTP relay.
type Sender struct {
	addr string // host:port
	from string
	auth smtp.Auth
}

// New builds a Sender from a Config. With an empty Host the Sender is disabled
// (Send returns ErrDisabled and Enabled reports false).
func New(c Config) *Sender {
	if c.Host == "" {
		return &Sender{}
	}
	port := c.Port
	if port == 0 {
		port = 587
	}
	var a smtp.Auth
	if c.User != "" {
		a = smtp.PlainAuth("", c.User, c.Pass, c.Host)
	}
	return &Sender{addr: fmt.Sprintf("%s:%d", c.Host, port), from: c.From, auth: a}
}

// Enabled reports whether the Sender can send.
func (s *Sender) Enabled() bool { return s.addr != "" }

// Send delivers a plain-text email to a single recipient.
func (s *Sender) Send(to, subject, body string) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	msg := buildMessage(s.from, to, subject, body)
	if err := smtp.SendMail(s.addr, s.auth, senderAddress(s.from), []string{to}, msg); err != nil {
		return fmt.Errorf("mail: send to %q: %w", to, err)
	}
	return nil
}

// BuildMessage assembles an RFC 5322 plain-text message (CRLF line endings). It
// is exported so callers can inspect/test the wire format without an SMTP relay.
func BuildMessage(from, to, subject, body string) []byte {
	return buildMessage(from, to, subject, body)
}

func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(b.String())
}

// senderAddress extracts the bare address from a "Name <addr>" From header,
// which is what the SMTP MAIL FROM command needs.
func senderAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j >= 0 {
			return from[i+1 : i+j]
		}
	}
	return from
}

// SenderAddress is the exported form of the MAIL FROM extraction (testable).
func SenderAddress(from string) string { return senderAddress(from) }
