package migrate

import (
	"context"
	"os"
	"slices"
	"testing"
	"testing/fstest"

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

func TestUpAppliesPendingAndIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations, mtest_a, mtest_b")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations, mtest_a, mtest_b") })

	fsys := fstest.MapFS{
		"000001_a.up.sql":   {Data: []byte("CREATE TABLE mtest_a (id int);")},
		"000001_a.down.sql": {Data: []byte("DROP TABLE mtest_a;")}, // must be ignored
		"000002_b.up.sql":   {Data: []byte("CREATE TABLE mtest_b (id int);")},
	}

	if err := Up(ctx, pool, fsys); err != nil {
		t.Fatalf("Up: %v", err)
	}
	got, err := Applied(ctx, pool)
	if err != nil {
		t.Fatalf("Applied: %v", err)
	}
	if want := []string{"000001_a", "000002_b"}; !slices.Equal(got, want) {
		t.Fatalf("applied = %v, want %v", got, want)
	}

	// Idempotent: a second run applies nothing and errors not.
	if err := Up(ctx, pool, fsys); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	got2, _ := Applied(ctx, pool)
	if !slices.Equal(got, got2) {
		t.Errorf("applied changed on re-run: %v -> %v", got, got2)
	}
}
