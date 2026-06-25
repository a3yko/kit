# billing/sumup/sumupgo

A ready-made [`billing/sumup.Charger`](../) backed by the official
[`sumup/sumup-go`](https://github.com/sumup/sumup-go) SDK.

It performs a merchant-initiated (saved-card) charge the way SumUp models it:
`Checkouts.Create` for the amount, then `Checkouts.Process` with the customer's
stored card token. The recurring scheduling/state lives in the parent package's
`Engine` + your `Store`; this type is only the "actually charge the card" step.

It's a **separate package** so importing the core orchestration
(`github.com/a3yko/kit/billing/sumup`) doesn't pull in the SDK — only code that
imports this adapter compiles against `sumup-go`.

## Install

```sh
go get github.com/a3yko/kit/billing/sumup/sumupgo
```

## Usage

```go
charger := sumupgo.New(apiKey, merchantCode)
engine  := sumup.NewEngine(charger, store)
engine.ProcessDue(ctx, time.Now())
```

If you already hold a configured SumUp client (e.g. with OAuth tokens or a custom
`http.Client`):

```go
charger := sumupgo.NewWithClient(client, merchantCode)
```

A synchronously-`PAID` checkout maps to `sumup.StatusSuccessful`; anything else
(accepted/pending/failed) is reported as not successful, so the `Engine` won't
advance the billing date.

Verified against `sumup-go` v0.17.0.
