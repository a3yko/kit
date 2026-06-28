// Package i18n is a tiny message-catalog engine with per-request locale on the
// context. A Bundle loads flattened key→string JSON catalogs from any fs.FS
// (embed them in your app, like kit/migrate reads migrations from an fs.FS), and
// lookups fall back to the default locale and finally the key itself — so a
// missing translation degrades visibly but never blanks the page.
//
// The HTTP policy (which header/cookie/param wins, whether to persist a choice)
// stays in your app; this package is the lookup engine + the context plumbing
// (WithLocale / Locale) that your middleware and templates share.
package i18n

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
)

// Locale is a selectable language: its code and own-language display name (the
// catalog's "language_name" key, if present).
type Locale struct {
	Code string
	Name string
}

// Bundle is a set of loaded catalogs with a default locale.
type Bundle struct {
	def       string
	catalogs  map[string]map[string]string
	available []Locale
}

// Load reads "<code>.json" for each code from fsys, in the given order. The first
// code is the default/fallback locale; order also fixes the language-switcher
// order. A code without a file is skipped. At least one catalog must load.
func Load(fsys fs.FS, order ...string) (*Bundle, error) {
	b := &Bundle{catalogs: map[string]map[string]string{}}
	for _, code := range order {
		raw, err := fs.ReadFile(fsys, code+".json")
		if err != nil {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		b.catalogs[code] = m
		name := m["language_name"]
		if name == "" {
			name = code
		}
		b.available = append(b.available, Locale{Code: code, Name: name})
		if b.def == "" {
			b.def = code
		}
	}
	if len(b.catalogs) == 0 {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

// Default returns the fallback locale code.
func (b *Bundle) Default() string { return b.def }

// Available lists the loaded locales in their declared order.
func (b *Bundle) Available() []Locale { return b.available }

// Supported reports whether a catalog exists for the code.
func (b *Bundle) Supported(code string) bool { _, ok := b.catalogs[code]; return ok }

// T returns the translation of key in locale, falling back to the default locale
// and then the key itself.
func (b *Bundle) T(locale, key string) string {
	if c, ok := b.catalogs[locale]; ok {
		if v, ok := c[key]; ok {
			return v
		}
	}
	if c, ok := b.catalogs[b.def]; ok {
		if v, ok := c[key]; ok {
			return v
		}
	}
	return key
}

// Tf is T with Rails-style %{name} interpolation.
func (b *Bundle) Tf(locale, key string, vars map[string]string) string {
	s := b.T(locale, key)
	for k, val := range vars {
		s = strings.ReplaceAll(s, "%{"+k+"}", val)
	}
	return s
}

// Resolve picks the best locale from candidates in priority order (first
// supported wins), falling back to the default. Empty candidates are skipped, and
// an Accept-Language-style value ("bg-BG,bg;q=0.9") is tried by its primary
// subtag too.
func (b *Bundle) Resolve(candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if b.Supported(c) {
			return c
		}
		if i := strings.IndexAny(c, "-,;"); i > 0 && b.Supported(c[:i]) {
			return c[:i]
		}
	}
	return b.def
}

// TCtx translates key for the locale carried on ctx (or the default).
func (b *Bundle) TCtx(ctx context.Context, key string) string { return b.T(LocaleOr(ctx, b.def), key) }

// TfCtx is TCtx with %{name} interpolation.
func (b *Bundle) TfCtx(ctx context.Context, key string, vars map[string]string) string {
	return b.Tf(LocaleOr(ctx, b.def), key, vars)
}

type ctxKey struct{}

// WithLocale returns a context carrying the resolved locale code.
func WithLocale(ctx context.Context, code string) context.Context {
	return context.WithValue(ctx, ctxKey{}, code)
}

// FromContext returns the context's locale code and whether one was set.
func FromContext(ctx context.Context) (string, bool) {
	if ctx != nil {
		if v, ok := ctx.Value(ctxKey{}).(string); ok && v != "" {
			return v, true
		}
	}
	return "", false
}

// LocaleOr returns the context's locale code, or def when none is set.
func LocaleOr(ctx context.Context, def string) string {
	if v, ok := FromContext(ctx); ok {
		return v
	}
	return def
}
