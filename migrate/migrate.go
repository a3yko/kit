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
	"sort"
	"strings"

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
