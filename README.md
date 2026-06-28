# kit

Small, reusable Go building blocks I use across my SaaS apps (server-rendered
Go + [Datastar](https://data-star.dev), PostgreSQL, SumUp billing).

It's **public and MIT-licensed** — if it's useful to you, take it. But it exists
for my own apps: **no support, no stability guarantees, no roadmap, breaking
changes whenever I need them.** Pin a commit if you depend on it.

## Packages

| Package | What it does | Status |
|---------|--------------|--------|
| `auth` | Session-based auth: bcrypt password hash/verify, random tokens, and a cookie-backed session `Manager` + middleware. You implement `SessionStore`; sessions key off an opaque user id. | early |
| `tenancy` | Resolve the current tenant from a request — custom domain → subdomain — onto the context, with middleware. You implement the `Lookup`. | early |
| `authz` | Tiny generic role→permission core (`Roles[R].Can/CanAny/CanAll`). Resource-scoped rules stay in your app and compose with a permission check. | early |
| `migrate` | Dependency-light SQL migration runner over an `fs.FS` of `*.up.sql` files — Rails' `db:migrate`, DB-only. | early |
| `jobs` | Postgres-backed background job queue + worker (`FOR UPDATE SKIP LOCKED`, retries w/ backoff, lease-based crash recovery) — Rails' Solid Queue, no Redis. Run-later scheduling, priorities, deduplicated enqueue (`WithUnique`) and self-rescheduling periodic jobs (`RegisterPeriodic`) for cron-free recurring work. | early |
| `cache` | Postgres-backed key/value cache with TTL — Rails' Solid Cache, no Redis. | early |
| `log` | Structured request/job logging on `log/slog`: a process logger plus context propagation (request_id/user_id/tenant_id/job_id) and `net/http` middleware that stamps a request id and logs one line per request. Keeps the SSE `Flusher` passthrough Datastar needs. | early |
| `mail` | Tiny SMTP sender for transactional email: one `Send(to, subject, body)`, PLAIN auth, graceful-disable when no host is set (returns `ErrDisabled` so callers can fall back to e.g. a copyable link). | early |
| `money` | Money as integer minor units: precise decimal-string `Cents` parsing (no float rounding, half-up), `Decimal` for input values, and symbol+separator `Format` for display. One place to turn "1234.56" into 123456 and back. | early |
| `i18n` | Message-catalog engine: a `Bundle` loads flattened key→string JSON catalogs from any `fs.FS`, with default-locale + key fallback, `Resolve` (param/cookie/Accept-Language), `%{name}` interpolation, and context plumbing (`WithLocale`/`FromContext`). HTTP policy stays in your app. | early |
| `dbx` | The small repetitive helpers for building sqlc/pgx params from Go values: `UUID`/`NullUUID`/`Date`/`Timestamptz` pgtype wrappers and generic `Ptr`/`Deref`/`DerefOr` for nullable columns. | early |
| `billing/sumup` | Recurring-subscription + saved-card (merchant-initiated) billing **orchestration** on top of the official [`sumup/sumup-go`](https://github.com/sumup/sumup-go) SDK — the bit the SDK deliberately doesn't do. SDK-free core: recurring `Engine` (you implement `Charger`/`Store`), `Interval` cadences (weekly→yearly) and `VerifyWebhook` (HMAC-SHA256). `billing/sumup/sumupgo` is the SDK-backed side: a `Charger` plus a `Client` for the GA saved-card flow — create a customer, open a CHECKOUT/SETUP_RECURRING_PAYMENT checkout for the card widget or hosted page, read back the tokenized card, list/delete saved cards. | early |
| `datastarx` | Minimal Server-Sent-Events helpers for driving [Datastar](https://data-star.dev) responses from `net/http`. | early |
| `storage` | Thin S3-compatible object-store wrapper (Cloudflare R2 / AWS S3 / MinIO): put, get, delete, presigned GET/PUT URLs. Built on aws-sdk-go-v2 with the checksum fix R2 needs. | early |

> **Dependency weight:** `billing/sumup/sumupgo` pulls in `sumup-go` and
> `storage` pulls in `aws-sdk-go-v2`. They live behind their own import paths, so
> the Go build only compiles what you import — but as one module they all share
> `go.sum`. If that bloat ever matters, the heavy adapters can move to their own
> modules later.

## Design notes

- **Thin, not a framework.** Each package fills a specific gap and stays out of
  your way. `billing/sumup` is orchestration *over* the official SDK, not another
  API client; `datastarx` is SSE plumbing, not a full Datastar wrapper.
- **Decoupled via interfaces.** Nothing here hard-binds your database or HTTP
  router — you implement small interfaces (`Store`, `Charger`, …) and the kit
  drives the workflow.

## Install

```sh
go get github.com/a3yko/kit@latest
```

## License

[MIT](LICENSE).
