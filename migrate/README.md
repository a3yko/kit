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
- `*.down.sql` files are ignored (kept for your own rollbacks via another tool).
- A failing migration rolls back its transaction and stops the run.
- `migrate.Applied(ctx, pool)` returns the applied versions, oldest-first.

Pairs with the `migrate` CLI for authoring/rollback in dev; this runner is what
you call on boot/deploy so the binary is self-sufficient (no external CLI).
