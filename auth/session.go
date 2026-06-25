package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNoSession is returned by Authenticate when there is no valid session.
var ErrNoSession = errors.New("auth: no session")

// Session is a stored login session. ID is the opaque value held in the cookie.
type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}

// Expired reports whether the session is at or past its expiry.
func (s Session) Expired(now time.Time) bool { return !now.Before(s.ExpiresAt) }

// SessionStore persists sessions. Implement it against your database or cache.
// Find must return ErrNoSession when the id is unknown.
type SessionStore interface {
	Save(ctx context.Context, s Session) error
	Find(ctx context.Context, id string) (Session, error)
	Delete(ctx context.Context, id string) error
}

// Manager issues and validates cookie-backed sessions.
type Manager struct {
	store      SessionStore
	cookieName string
	ttl        time.Duration
	secure     bool
}

// Option configures a Manager.
type Option func(*Manager)

// WithCookieName sets the session cookie name (default "session"). Use distinct
// names for separate scopes, e.g. "session" and "admin_session".
func WithCookieName(name string) Option { return func(m *Manager) { m.cookieName = name } }

// WithTTL sets the session lifetime (default 30 days).
func WithTTL(d time.Duration) Option { return func(m *Manager) { m.ttl = d } }

// Insecure drops the cookie's Secure flag so it works over plain HTTP. Use it
// only in local development.
func Insecure() Option { return func(m *Manager) { m.secure = false } }

// NewManager builds a session manager. Cookies default to Secure, HttpOnly,
// SameSite=Lax, path "/", named "session", with a 30-day TTL.
func NewManager(store SessionStore, opts ...Option) *Manager {
	m := &Manager{store: store, cookieName: "session", ttl: 30 * 24 * time.Hour, secure: true}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Issue creates a session for userID, persists it, and writes the cookie.
func (m *Manager) Issue(ctx context.Context, w http.ResponseWriter, userID string) (Session, error) {
	id, err := NewToken()
	if err != nil {
		return Session{}, err
	}
	s := Session{ID: id, UserID: userID, ExpiresAt: time.Now().Add(m.ttl)}
	if err := m.store.Save(ctx, s); err != nil {
		return Session{}, err
	}
	http.SetCookie(w, m.cookie(id, m.ttl))
	return s, nil
}

// Authenticate reads the session cookie and returns the valid session. It
// returns ErrNoSession when there is no cookie, no matching session, or the
// session has expired (expired sessions are deleted). Unexpected store errors
// are returned wrapped, so a backing-store outage surfaces as an error rather
// than masquerading as a logged-out user — check with errors.Is(err, ErrNoSession).
func (m *Manager) Authenticate(ctx context.Context, r *http.Request) (Session, error) {
	c, err := r.Cookie(m.cookieName)
	if err != nil {
		return Session{}, ErrNoSession
	}
	s, err := m.store.Find(ctx, c.Value)
	switch {
	case errors.Is(err, ErrNoSession):
		return Session{}, ErrNoSession
	case err != nil:
		return Session{}, fmt.Errorf("auth: load session: %w", err)
	}
	if s.Expired(time.Now()) {
		_ = m.store.Delete(ctx, s.ID)
		return Session{}, ErrNoSession
	}
	return s, nil
}

// Revoke deletes the current session (if any) and clears the cookie.
func (m *Manager) Revoke(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(m.cookieName); err == nil {
		if err := m.store.Delete(ctx, c.Value); err != nil {
			return err
		}
	}
	http.SetCookie(w, m.cookie("", -time.Hour))
	return nil
}

func (m *Manager) cookie(value string, maxAge time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     m.cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
	}
}
