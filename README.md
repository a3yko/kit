# kit

Small, reusable Go building blocks I use across my SaaS apps (server-rendered
Go + [Datastar](https://data-star.dev), PostgreSQL, SumUp billing).

It's **public and MIT-licensed** — if it's useful to you, take it. But it exists
for my own apps: **no support, no stability guarantees, no roadmap, breaking
changes whenever I need them.** Pin a commit if you depend on it.

## Packages

| Package | What it does | Status |
|---------|--------------|--------|
| `billing/sumup` | Recurring-subscription + saved-card (merchant-initiated) billing **orchestration** on top of the official [`sumup/sumup-go`](https://github.com/sumup/sumup-go) SDK — the bit the SDK deliberately doesn't do. | early |
| `datastarx` | Minimal Server-Sent-Events helpers for driving [Datastar](https://data-star.dev) responses from `net/http`. | early |

Planned (extracted as my apps need them): `auth`, `tenancy`, `storage` (R2/S3).

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
