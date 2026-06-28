package i18n

import (
	"context"
	"testing"
	"testing/fstest"
)

func testBundle(t *testing.T) *Bundle {
	t.Helper()
	fsys := fstest.MapFS{
		"en.json": {Data: []byte(`{"language_name":"English","nav.team":"Team","hi":"Hi %{name}"}`)},
		"bg.json": {Data: []byte(`{"language_name":"Български","nav.team":"Екип"}`)},
	}
	b, err := Load(fsys, "en", "bg")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return b
}

func TestLoadAndAvailable(t *testing.T) {
	b := testBundle(t)
	if b.Default() != "en" {
		t.Errorf("default = %q, want en", b.Default())
	}
	av := b.Available()
	if len(av) != 2 || av[0].Code != "en" || av[0].Name != "English" || av[1].Name != "Български" {
		t.Errorf("available = %+v", av)
	}
}

func TestTranslateFallback(t *testing.T) {
	b := testBundle(t)
	if got := b.T("bg", "nav.team"); got != "Екип" {
		t.Errorf("bg nav.team = %q", got)
	}
	// Key present in en only → bg falls back to en.
	if got := b.T("bg", "hi"); got != "Hi %{name}" {
		t.Errorf("bg fallback = %q", got)
	}
	// Unknown locale → default.
	if got := b.T("fr", "nav.team"); got != "Team" {
		t.Errorf("fr fallback = %q", got)
	}
	// Unknown key → the key itself.
	if got := b.T("en", "nope"); got != "nope" {
		t.Errorf("missing key = %q", got)
	}
}

func TestInterpolation(t *testing.T) {
	if got := testBundle(t).Tf("en", "hi", map[string]string{"name": "Ada"}); got != "Hi Ada" {
		t.Errorf("Tf = %q, want Hi Ada", got)
	}
}

func TestResolve(t *testing.T) {
	b := testBundle(t)
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"bg"}, "bg"},
		{[]string{"", "en"}, "en"},
		{[]string{"fr", "bg"}, "bg"},
		{[]string{"bg-BG,bg;q=0.9"}, "bg"},
		{[]string{"de", "es"}, "en"},
	}
	for _, c := range cases {
		if got := b.Resolve(c.in...); got != c.want {
			t.Errorf("Resolve(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestContext(t *testing.T) {
	b := testBundle(t)
	if _, ok := FromContext(context.Background()); ok {
		t.Error("empty context should have no locale")
	}
	ctx := WithLocale(context.Background(), "bg")
	if v, ok := FromContext(ctx); !ok || v != "bg" {
		t.Errorf("Locale = %q,%v", v, ok)
	}
	if got := b.TCtx(ctx, "nav.team"); got != "Екип" {
		t.Errorf("TCtx(bg) = %q", got)
	}
	if got := b.TCtx(context.Background(), "nav.team"); got != "Team" {
		t.Errorf("TCtx(default) = %q", got)
	}
}
