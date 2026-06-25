# migrate

A tiny, dependency-light SQL migration runner — Rails' `db:migrate`, DB-only.
It applies the `*.up.sql` files from an `fs.FS` (embed them in your binary), in
filename order, each in its own transaction, tracking applied versions in a
`schema_migrations` table.

## Install

```sh
go get github.com/a3yko/kit/migrate
```

## Usage

```go
//go:embed db/migrations/*.sql
var migrations embed.FS

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
    sub, err := fs.Sub(migrations, "db/migrations")
    if err != nil {
        return err
    }
    return migrate.Up(ctx, pool, sub) // applies pending *.up.sql, idempotent
}
```

- Versions are the filename without `.up.sql` (e.g. `000001_catalog`).
- A failing migration rolls back its transaction and stops the run.
- `migrate.Applied(ctx, pool)` returns the applied versions, oldest-first.

## The rest of the Rails db: verbs

```go
migrate.Rollback(ctx, pool, fsys, 1) // revert the last n migrations via their .down.sql (db:rollback)
migrate.Reset(ctx, pool, fsys)       // drop the schema + re-apply all migrations, in place (db:reset)
migrate.Drop(ctx, pool)              // wipe the public schema (destructive; dev/test)

migrate.CreateDatabase(ctx, url)     // create the DB named in url if absent (db:create)
migrate.DropDatabase(ctx, url)       // drop it, terminating connections (db:drop)
```

`CreateDatabase`/`DropDatabase` take a connection **URL** (not a pool) — they
connect to the server's `postgres` maintenance database to act on the named one.
`Rollback`/`Reset`/`Drop` take the pool. The destructive ones are dev/test tools.

Pairs with the `migrate` CLI for authoring/rollback in dev; this runner is what
you call on boot/deploy so the binary is self-sufficient (no external CLI).
