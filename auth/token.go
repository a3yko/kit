package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// NewToken returns a cryptographically-random, URL-safe token with 32 bytes of
// entropy — suitable for session ids and password-reset / email-confirmation
// tokens.
func NewToken() (string, error) { return NewTokenN(32) }

// NewTokenN returns a URL-safe token with n bytes of entropy.
func NewTokenN(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// TokensEqual compares two tokens in constant time, so verifying a reset or
// confirmation token doesn't leak its value via timing.
func TokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
