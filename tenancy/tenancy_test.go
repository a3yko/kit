package tenancy

import (
	"context"
	"testing"
)

func TestSubdomain(t *testing.T) {
	tests := []struct {
		host, root string
		want       string
		ok         bool
	}{
		{"acme.app.com", "app.com", "acme", true},
		{"acme.app.com:443", "app.com", "acme", true},
		{"app.com", "app.com", "", false},        // apex
		{"www.app.com", "app.com", "", false},    // www excluded
		{"a.b.app.com", "app.com", "", false},    // multi-label
		{"acme.other.com", "app.com", "", false}, // wrong root
	}
	for _, tc := range tests {
		got, ok := Subdomain(tc.host, tc.root)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Subdomain(%q,%q) = (%q,%v), want (%q,%v)", tc.host, tc.root, got, ok, tc.want, tc.ok)
		}
	}
}

// fakeLookup resolves from two maps.
type fakeLookup struct {
	subs    map[string]string
	domains map[string]string
}

func (f fakeLookup) BySubdomain(_ context.Context, sub string) (string, error) {
	if id, ok := f.subs[sub]; ok {
		return id, nil
	}
	return "", ErrNoTenant
}
func (f fakeLookup) ByCustomDomain(_ context.Context, host string) (string, error) {
	if id, ok := f.domains[host]; ok {
		return id, nil
	}
	return "", ErrNoTenant
}

func TestResolvePrecedence(t *testing.T) {
	r := NewResolver("app.com", fakeLookup{
		subs:    map[string]string{"acme": "t-sub"},
		domains: map[string]string{"cars.acme.com": "t-domain"},
	})
	ctx := context.Background()

	if id, _ := r.Resolve(ctx, "cars.acme.com"); id != "t-domain" {
		t.Errorf("custom domain: got %q, want t-domain", id)
	}
	if id, _ := r.Resolve(ctx, "acme.app.com:443"); id != "t-sub" {
		t.Errorf("subdomain: got %q, want t-sub", id)
	}
	if _, err := r.Resolve(ctx, "nope.app.com"); err != ErrNoTenant {
		t.Errorf("unknown: err = %v, want ErrNoTenant", err)
	}
}
