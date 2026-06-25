package migrate

import (
	"context"
	"net/url"
	"os"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// twoMigrations is a throwaway up/down pair used by several tests.
var twoMigrations = fstest.MapFS{
	"000001_a.up.sql":   {Data: []byte("CREATE TABLE mtest_a (id int);")},
	"000001_a.down.sql": {Data: []byte("DROP TABLE mtest_a;")},
	"000002_b.up.sql":   {Data: []byte("CREATE TABLE mtest_b (id int);")},
	"000002_b.down.sql": {Data: []byte("DROP TABLE mtest_b;")},
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	err := pool.QueryRow(context.Background(),
		"SELECT to_regclass($1) IS NOT NULL", "public."+name).Scan(&ok)
	if err != nil {
		t.Fatalf("tableExists(%s): %v", name, err)
	}
	return ok
}

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

func TestRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations, mtest_a, mtest_b")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations, mtest_a, mtest_b") })

	if err := Up(ctx, pool, twoMigrations); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Roll back the newest only.
	if err := Rollback(ctx, pool, twoMigrations, 1); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if tableExists(t, pool, "mtest_b") {
		t.Error("mtest_b should be gone after rollback")
	}
	if !tableExists(t, pool, "mtest_a") {
		t.Error("mtest_a should remain after rolling back one step")
	}
	if got, _ := Applied(ctx, pool); !slices.Equal(got, []string{"000001_a"}) {
		t.Errorf("applied = %v, want [000001_a]", got)
	}

	// Roll back the rest.
	if err := Rollback(ctx, pool, twoMigrations, 5); err != nil {
		t.Fatalf("Rollback(rest): %v", err)
	}
	if tableExists(t, pool, "mtest_a") {
		t.Error("mtest_a should be gone after full rollback")
	}
}

func TestReset(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations, mtest_a, mtest_b") })

	if err := Up(ctx, pool, twoMigrations); err != nil {
		t.Fatalf("Up: %v", err)
	}
	_, _ = pool.Exec(ctx, "INSERT INTO mtest_a (id) VALUES (1)")

	if err := Reset(ctx, pool, twoMigrations); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !tableExists(t, pool, "mtest_a") || !tableExists(t, pool, "mtest_b") {
		t.Error("tables should exist after Reset")
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM mtest_a").Scan(&n)
	if n != 0 {
		t.Errorf("mtest_a rows = %d, want 0 (data wiped)", n)
	}
	if got, _ := Applied(ctx, pool); !slices.Equal(got, []string{"000001_a", "000002_b"}) {
		t.Errorf("applied = %v, want both", got)
	}
}

func TestCreateAndDropDatabase(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	// Derive a sibling DB URL on the same server.
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u.Path = "/kit_test_createdrop"
	target := u.String()
	ctx := context.Background()

	_ = DropDatabase(ctx, target) // clean slate
	if err := CreateDatabase(ctx, target); err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	// Idempotent.
	if err := CreateDatabase(ctx, target); err != nil {
		t.Errorf("CreateDatabase (second): %v", err)
	}
	// We can connect to it.
	conn, err := pgx.Connect(ctx, target)
	if err != nil {
		t.Fatalf("connect to created db: %v", err)
	}
	_ = conn.Close(ctx)

	if err := DropDatabase(ctx, target); err != nil {
		t.Fatalf("DropDatabase: %v", err)
	}
	if _, err := pgx.Connect(ctx, target); err == nil {
		t.Error("expected connect to fail after drop")
	}
}
