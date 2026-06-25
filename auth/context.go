package auth

import (
	"context"
	"net/http"
)

type ctxKey struct{}

// WithUserID returns a copy of ctx carrying the authenticated user id.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// UserID returns the authenticated user id from ctx, and whether one is present.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok && id != ""
}

// LoadUser is middleware that authenticates the request when a valid session
// exists and stores the user id on the context. It never blocks the request —
// pair it with RequireUser to enforce.
func (m *Manager) LoadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s, err := m.Authenticate(r.Context(), r); err == nil {
			r = r.WithContext(WithUserID(r.Context(), s.UserID))
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUser is middleware that responds 401 when no authenticated user is on
// the context (as set by LoadUser). For browser apps, swap it for a redirect to
// your login page.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserID(r.Context()); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
