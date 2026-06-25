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
| `billing/sumup` | Recurring-subscription + saved-card (merchant-initiated) billing **orchestration** on top of the official [`sumup/sumup-go`](https://github.com/sumup/sumup-go) SDK — the bit the SDK deliberately doesn't do. Core is SDK-free (you implement `Charger`/`Store`); `billing/sumup/sumupgo` is an example `Charger` backed by the SDK. | early |
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
