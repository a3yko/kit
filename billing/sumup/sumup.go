// Package sumup provides recurring-subscription and saved-card
// (merchant-initiated) billing orchestration on top of SumUp.
//
// The official SumUp Go SDK (github.com/sumup/sumup-go) is a raw, generated API
// client — checkouts, customers, transactions, etc. It deliberately does NOT
// handle recurring billing, subscription lifecycle, or merchant-initiated
// (saved-card) charges. This package fills exactly that gap and nothing else: a
// thin orchestration layer, not another API client.
//
// It is decoupled from both the SDK and your database:
//
//   - implement [Charger] to perform the actual charge (back it with sumup-go),
//   - implement [Store] to read due subscriptions and persist results,
//
// and [Engine] drives the loop: find due -> charge saved card -> record ->
// advance the next billing date.
package sumup

import (
	"context"
	"time"
)

// Interval is how often a subscription renews.
type Interval string

const (
	Monthly Interval = "monthly"
	Yearly  Interval = "yearly"
)

// Next returns the next billing time after t for this interval.
func (i Interval) Next(t time.Time) time.Time {
	if i == Yearly {
		return t.AddDate(1, 0, 0)
	}
	return t.AddDate(0, 1, 0) // default: monthly
}

// Subscription is the minimal view the engine needs of one renewable
// subscription. Your app maps its own table onto this.
type Subscription struct {
	ID            string   // your subscription / tenant id
	CustomerID    string   // SumUp customer id
	CardToken     string   // saved payment-instrument token (card on file)
	AmountCents   int      // amount to charge, in minor units
	Currency      string   // ISO 4217, e.g. "EUR"
	Interval      Interval // renewal cadence
	NextBillingAt time.Time
}

// ChargeRequest is a single merchant-initiated charge against a saved card.
type ChargeRequest struct {
	CustomerID  string
	CardToken   string
	AmountCents int
	Currency    string
	Reference   string // your idempotency / order reference
	Description string
}

// ChargeResult is the outcome of a charge attempt.
type ChargeResult struct {
	TransactionID string
	Status        string // e.g. "SUCCESSFUL", "FAILED"
}

// Successful reports whether the charge completed.
func (r ChargeResult) Successful() bool { return r.Status == "SUCCESSFUL" }

// Charger performs a merchant-initiated charge against a saved card. Back this
// with the official sumup-go SDK.
//
// TODO: confirm sumup-go exposes saved payment-instrument / MIT charging before
// wiring the concrete implementation. If it does not, drive the checkouts API
// directly with the stored card token inside the implementation.
type Charger interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}

// Store is your persistence boundary: which subscriptions are due, and how the
// results are recorded. Implement it against your own database.
type Store interface {
	// DueForRenewal returns auto-renewing subscriptions whose NextBillingAt <= now.
	DueForRenewal(ctx context.Context, now time.Time) ([]Subscription, error)

	// RecordCharge persists the outcome of a charge attempt (chargeErr is the
	// error returned by Charger, or nil on success).
	RecordCharge(ctx context.Context, sub Subscription, res ChargeResult, chargeErr error) error

	// Advance moves the subscription's next billing date forward after success.
	Advance(ctx context.Context, sub Subscription, next time.Time) error
}
