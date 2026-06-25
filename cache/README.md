# cache

A Postgres-backed key/value cache — Rails' Solid Cache, no Redis. Values are
bytes with an optional TTL; expired entries are ignored on read and removed by
`Purge`.

## Install

```sh
go get github.com/a3yko/kit/cache
```

## Usage

```go
c := cache.New(pool)
if err := c.EnsureSchema(ctx); err != nil { /* ... */ }

c.Set(ctx, "user:42:profile", data, 10*time.Minute) // ttl <= 0 means no expiry

if v, ok, err := c.Get(ctx, "user:42:profile"); err == nil && ok {
    use(v)
}

c.Delete(ctx, "user:42:profile")

// Sweep expired rows periodically (e.g. a recurring jobs.Queue task):
removed, _ := c.Purge(ctx)
```

## Notes

- Values are `[]byte` — marshal/compress as you like before `Set`.
- `Get` transparently treats expired entries as absent (`found == false`);
  nothing is auto-deleted on read, so schedule `Purge` to reclaim space.
- Keys are unique; `Set` upserts.
