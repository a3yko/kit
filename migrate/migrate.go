// Package migrate is a tiny, dependency-light SQL migration runner — Rails'
// db:migrate, DB-only. It applies the *.up.sql files from an fs.FS (embed them
// in your binary), in filename order, each in its own transaction, tracking
// applied versions in a schema_migrations table (version = filename without the
// .up.sql suffix, e.g. "000001_catalog").
//
//	//go:embed db/migrations/*.sql
//	var migrations embed.FS
//	// ...
//	sub, _ := fs.Sub(migrations, "db/migrations")
//	if err := migrate.Up(ctx, pool, sub); err != nil { ... }
package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ensureTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    text PRIMARY KEY,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

// Up applies every pending *.up.sql migration in fsys, sorted by filename. It is
// idempotent: already-applied versions are skipped. A failing migration stops
// the run (its transaction rolls back; later migrations are not applied).
func Up(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	if _, err := pool.Exec(ctx, ensureTable); err != nil {
		return fmt.Errorf("migrate: ensure schema_migrations: %w", err)
	}

	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("migrate: list migrations: %w", err)
	}
	sort.Strings(files)

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, f := range files {
		version := strings.TrimSuffix(f, ".up.sql")
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(fsys, f)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", f, err)
		}
		if err := applyOne(ctx, pool, version, string(body)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", version, err)
		}
	}
	return nil
}

// Applied returns the set of versions already applied, oldest-first.
func Applied(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	versions, err := Applied(ctx, pool)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(versions))
	for _, v := range versions {
		set[v] = true
	}
	return set, nil
}

// Rollback reverts the most recently applied migrations by running their
// matching <version>.down.sql files, newest first, removing each from
// schema_migrations. steps <= 0 is treated as 1 (Rails' db:rollback). It errors
// if a needed .down.sql is missing.
func Rollback(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, steps int) error {
	if steps <= 0 {
		steps = 1
	}
	applied, err := Applied(ctx, pool)
	if err != nil {
		return err
	}
	for i := 0; i < steps && len(applied) > 0; i++ {
		version := applied[len(applied)-1]
		applied = applied[:len(applied)-1]

		body, err := fs.ReadFile(fsys, version+".down.sql")
		if err != nil {
			return fmt.Errorf("migrate: read down %s: %w", version, err)
		}
		if err := revertOne(ctx, pool, version, string(body)); err != nil {
			return fmt.Errorf("migrate: rollback %s: %w", version, err)
		}
	}
	return nil
}

func revertOne(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Drop wipes the public schema (every table, including schema_migrations) by
// recreating it. Destructive — dev/test only.
func Drop(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		return fmt.Errorf("migrate: drop schema: %w", err)
	}
	return nil
}

// Reset drops the public schema and re-applies all migrations — Rails'
// db:reset (in place; the database itself is kept). Destructive, dev/test only.
func Reset(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	if err := Drop(ctx, pool); err != nil {
		return err
	}
	return Up(ctx, pool, fsys)
}

// CreateDatabase creates the database named in dbURL if it does not exist, by
// connecting to the server's "postgres" maintenance database — Rails' db:create.
func CreateDatabase(ctx context.Context, dbURL string) error {
	adminURL, name, err := maintenanceURL(dbURL)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("migrate: connect maintenance db: %w", err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT exists(SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return fmt.Errorf("migrate: check database: %w", err)
	}
	if exists {
		return nil
	}
	// DDL can't be parameterized; Sanitize quotes the identifier safely.
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("migrate: create database %q: %w", name, err)
	}
	return nil
}

// DropDatabase drops the database named in dbURL if it exists, terminating any
// open connections — Rails' db:drop. Destructive, dev/test only.
func DropDatabase(ctx context.Context, dbURL string) error {
	adminURL, name, err := maintenanceURL(dbURL)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("migrate: connect maintenance db: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("migrate: drop database %q: %w", name, err)
	}
	return nil
}

// maintenanceURL splits a Postgres URL into a connection string pointing at the
// server's "postgres" database and the target database name.
func maintenanceURL(dbURL string) (adminURL, dbName string, err error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return "", "", fmt.Errorf("migrate: parse url: %w", err)
	}
	dbName = strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", "", fmt.Errorf("migrate: no database name in url")
	}
	u.Path = "/postgres"
	return u.String(), dbName, nil
}
