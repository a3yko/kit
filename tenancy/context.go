package tenancy

import "context"

type ctxKey struct{}

// WithTenantID returns a copy of ctx carrying the resolved tenant id.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// TenantID returns the resolved tenant id from ctx, and whether one is present.
func TenantID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok && id != ""
}
