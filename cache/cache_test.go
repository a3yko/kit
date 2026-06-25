package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestCache(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS cache_entries")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS cache_entries") })

	c := New(pool)
	if err := c.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	// Set/Get round trip.
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, ok, _ := c.Get(ctx, "k"); !ok || string(v) != "v" {
		t.Errorf("Get(k) = (%q,%v), want (v,true)", v, ok)
	}

	// Missing key.
	if _, ok, _ := c.Get(ctx, "missing"); ok {
		t.Error("Get(missing) should not be found")
	}

	// Expiry: a past TTL is never returned, and Purge removes it.
	if err := c.Set(ctx, "e", []byte("x"), time.Millisecond); err != nil {
		t.Fatalf("Set(e): %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, "e"); ok {
		t.Error("expired key should not be found")
	}
	if n, _ := c.Purge(ctx); n < 1 {
		t.Errorf("Purge removed %d, want >= 1", n)
	}

	// Delete.
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Error("deleted key should not be found")
	}
}
