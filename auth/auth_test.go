package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(hash, "s3cret") {
		t.Error("correct password failed to verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("wrong password verified")
	}
}

func TestTokensUniqueAndComparable(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	if a == "" || a == b {
		t.Errorf("tokens should be non-empty and unique: %q %q", a, b)
	}
	if !TokensEqual(a, a) || TokensEqual(a, b) {
		t.Error("TokensEqual is wrong")
	}
}

// memStore is an in-memory SessionStore for tests.
type memStore map[string]Session

func (m memStore) Save(_ context.Context, s Session) error { m[s.ID] = s; return nil }
func (m memStore) Delete(_ context.Context, id string) error {
	delete(m, id)
	return nil
}
func (m memStore) Find(_ context.Context, id string) (Session, error) {
	if s, ok := m[id]; ok {
		return s, nil
	}
	return Session{}, ErrNoSession
}

func TestSessionLifecycle(t *testing.T) {
	mgr := NewManager(memStore{}, Insecure())

	// Issue writes a cookie.
	w := httptest.NewRecorder()
	if _, err := mgr.Issue(context.Background(), w, "user-1"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	cookie := w.Result().Cookies()[0]

	// Authenticate reads it back.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)
	s, err := mgr.Authenticate(context.Background(), r)
	if err != nil || s.UserID != "user-1" {
		t.Fatalf("Authenticate = (%+v, %v), want user-1", s, err)
	}

	// Revoke clears it.
	w2 := httptest.NewRecorder()
	if err := mgr.Revoke(context.Background(), w2, r); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := mgr.Authenticate(context.Background(), r); err != ErrNoSession {
		t.Errorf("after Revoke, Authenticate err = %v, want ErrNoSession", err)
	}
}

// errStore returns a non-ErrNoSession failure from Find.
type errStore struct{ err error }

func (s errStore) Save(context.Context, Session) error           { return nil }
func (s errStore) Delete(context.Context, string) error          { return nil }
func (s errStore) Find(context.Context, string) (Session, error) { return Session{}, s.err }

func TestAuthenticatePropagatesStoreErrors(t *testing.T) {
	boom := errors.New("db down")
	mgr := NewManager(errStore{err: boom}, Insecure())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "x"})

	_, err := mgr.Authenticate(context.Background(), r)
	if errors.Is(err, ErrNoSession) || !errors.Is(err, boom) {
		t.Errorf("store error should propagate (wrapped), not become ErrNoSession; got %v", err)
	}
}

func TestRequireUserMiddleware(t *testing.T) {
	guarded := RequireUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No user → 401.
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without user: code = %d, want 401", w.Code)
	}

	// With user in context → 200.
	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(WithUserID(context.Background(), "u"))
	guarded.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("with user: code = %d, want 200", w.Code)
	}
}
