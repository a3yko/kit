# billing/sumup

Recurring-subscription + saved-card (merchant-initiated) billing **orchestration**
on top of the official [`sumup/sumup-go`](https://github.com/sumup/sumup-go) SDK.

The SDK is a raw API client — it deliberately does **not** do recurring billing,
subscription lifecycle, or saved-card charges. This package fills exactly that
gap: the "find due → charge the saved card → record → advance the billing date"
loop, decoupled from both the SDK and your database.

You implement two small interfaces:

- **`Store`** — read due subscriptions, persist results, advance billing dates.
- **`Charger`** — perform one charge. A ready-made one backed by `sumup-go` lives
  in [`sumupgo`](./sumupgo).

## Install

```sh
go get github.com/a3yko/kit/billing/sumup
```

## Wire it up

```go
import (
    "github.com/a3yko/kit/billing/sumup"
    "github.com/a3yko/kit/billing/sumup/sumupgo"
)

charger := sumupgo.New(apiKey, merchantCode) // example Charger over sumup-go
engine  := sumup.NewEngine(charger, store)    // store is your DB (implements sumup.Store)
```

## Run it on a schedule

```go
// In an hourly worker / ticker:
n, err := engine.ProcessDue(ctx, time.Now())
// n = successful charges; err = joined failures (one bad subscription never
// aborts the batch). Charges run sequentially by design.
```

`ProcessDue` for each due subscription: charges the saved card, calls
`Store.RecordCharge` (always — even on failure), and on success calls
`Store.Advance` with `Interval.Next(NextBillingAt)`.

## What you implement

```go
type Store interface {
    DueForRenewal(ctx context.Context, now time.Time) ([]sumup.Subscription, error)
    RecordCharge(ctx context.Context, sub sumup.Subscription, res sumup.ChargeResult, chargeErr error) error
    Advance(ctx context.Context, sub sumup.Subscription, next time.Time) error
}

type Charger interface { // see sumupgo for a sumup-go-backed implementation
    Charge(ctx context.Context, req sumup.ChargeRequest) (sumup.ChargeResult, error)
}
```

A `Subscription` carries what a charge needs: `CustomerID`, `CardToken` (the saved
card), `AmountCents`, `Currency`, `Interval` (`Monthly`/`Yearly`), `NextBillingAt`.
A `Charger` sets `ChargeResult.Status = sumup.StatusSuccessful` on success.

## Setting up the card (first payment)

Capturing the saved-card token is the *initial* checkout, done with the SumUp
widget + `sumup-go` (`Checkouts.Create` with `Purpose: SETUP_RECURRING_PAYMENT`,
then `Process` with a `Mandate`). Store the resulting customer id + card token on
your subscription; from then on this package handles the recurring charges.
