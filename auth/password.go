// Package auth provides reusable building blocks for session-based
// authentication: password hashing, cryptographically-random tokens, and a
// cookie-backed session manager with middleware.
//
// The mechanism lives here; your app supplies only storage (the small
// [SessionStore] interface) and decides what a "user" is — sessions key off an
// opaque user id string. The same primitives back both a normal user login and
// a separate platform-admin login (use a second Manager with its own cookie
// name and SessionStore).
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of plain, suitable for storage.
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword reports whether plain matches a bcrypt hash from HashPassword.
// The bcrypt comparison is constant-time.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
