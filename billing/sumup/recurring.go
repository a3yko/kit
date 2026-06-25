package sumup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Engine drives recurring billing: find due subscriptions, charge the saved
// card, record the result, and advance the next billing date on success.
type Engine struct {
	charger Charger
	store   Store
}

// NewEngine builds a recurring billing engine from a Charger and a Store.
func NewEngine(charger Charger, store Store) *Engine {
	return &Engine{charger: charger, store: store}
}

// ProcessDue charges every subscription due at now. A failure on one
// subscription is recorded and skipped — it never aborts the rest. It returns
// the number of successful charges and a joined error of any failures, so the
// caller (e.g. an hourly worker) can log/alert without losing the batch.
func (e *Engine) ProcessDue(ctx context.Context, now time.Time) (succeeded int, err error) {
	due, derr := e.store.DueForRenewal(ctx, now)
	if derr != nil {
		return 0, fmt.Errorf("sumup: load due subscriptions: %w", derr)
	}

	var errs []error
	for _, sub := range due {
		res, chargeErr := e.charger.Charge(ctx, ChargeRequest{
			CustomerID:  sub.CustomerID,
			CardToken:   sub.CardToken,
			AmountCents: sub.AmountCents,
			Currency:    sub.Currency,
			Reference:   fmt.Sprintf("%s-%d", sub.ID, sub.NextBillingAt.Unix()),
			Description: "Subscription renewal",
		})

		// Always record the attempt first; if we can't even persist it, surface
		// that and move on rather than risk double-charging on the next run.
		if recErr := e.store.RecordCharge(ctx, sub, res, chargeErr); recErr != nil {
			errs = append(errs, fmt.Errorf("record charge for %s: %w", sub.ID, recErr))
			continue
		}

		if chargeErr != nil {
			errs = append(errs, fmt.Errorf("charge %s: %w", sub.ID, chargeErr))
			continue
		}
		if !res.Successful() {
			errs = append(errs, fmt.Errorf("charge %s: status %q", sub.ID, res.Status))
			continue
		}

		if advErr := e.store.Advance(ctx, sub, sub.Interval.Next(sub.NextBillingAt)); advErr != nil {
			errs = append(errs, fmt.Errorf("advance %s: %w", sub.ID, advErr))
			continue
		}
		succeeded++
	}

	return succeeded, errors.Join(errs...)
}
