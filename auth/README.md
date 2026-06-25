# auth

Session-based authentication building blocks: bcrypt password hashing,
cryptographically-random tokens, and a cookie-backed session manager with
middleware.

The mechanism lives here; **you supply storage** (the small `SessionStore`
interface) and decide what a "user" is — sessions key off an opaque user id
string. The same primitives back both a normal user login and a separate
platform-admin login: use a second `Manager` with its own cookie name and store.

## Install

```sh
go get github.com/a3yko/kit/auth
```

## What you implement

```go
// Back this with your database (or a cache like Redis/Solid Cache).
type SessionStore interface {
    Save(ctx context.Context, s auth.Session) error
    Find(ctx context.Context, id string) (auth.Session, error) // return auth.ErrNoSession if absent
    Delete(ctx context.Context, id string) error
}
```

## Usage

### Registering / logging in

```go
// On signup: store the hash, never the password.
hash, _ := auth.HashPassword(form.Password)
user.PasswordHash = hash

// On login: verify, then issue a session (writes the cookie).
if !auth.VerifyPassword(user.PasswordHash, form.Password) {
    http.Error(w, "invalid credentials", http.StatusUnauthorized)
    return
}
mgr := auth.NewManager(sessionStore) // Secure, HttpOnly, SameSite=Lax, 30d
mgr.Issue(r.Context(), w, user.ID)
```

### Protecting routes

```go
// LoadUser populates the context if a valid session exists; RequireUser enforces it.
protected := mgr.LoadUser(auth.RequireUser(appHandler))

// Inside a handler:
uid, ok := auth.UserID(r.Context())
```

### Logging out

```go
mgr.Revoke(r.Context(), w, r) // deletes the session + clears the cookie
```

### Options

```go
auth.NewManager(store,
    auth.WithCookieName("admin_session"), // distinct scope
    auth.WithTTL(7*24*time.Hour),
    auth.Insecure(),                       // dev only: allow non-HTTPS cookies
)
```

## Tokens (password reset / email confirmation)

```go
tok, _ := auth.NewToken()          // store a hash of this, email the value
// ...later, on the confirm link:
ok := auth.TokensEqual(stored, provided) // constant-time
```

## Notes

- `RequireUser` returns **401**; for browser apps wrap/replace it with a redirect
  to your login page.
- An `auth.Session` knows only `ID`, `UserID`, `ExpiresAt`. Turning `UserID` into
  your `*User` is one small app-side lookup.
