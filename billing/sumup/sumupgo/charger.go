// Package sumupgo is an example [billing/sumup.Charger] backed by the official
// SumUp Go SDK (github.com/sumup/sumup-go).
//
// It performs a merchant-initiated (saved-card) charge the way SumUp's API
// models it: create a checkout for the amount, then process that checkout with
// the customer's stored card token. The recurring scheduling/state lives in the
// parent package's Engine + your Store; this type is only the "actually charge
// the card" boundary.
//
// It is a separate package so importing the core orchestration
// (github.com/a3yko/kit/billing/sumup) does not drag in the SDK — only code that
// imports this adapter compiles against sumup-go.
package sumupgo

import (
	"context"
	"fmt"

	billing "github.com/a3yko/kit/billing/sumup"
	sumup "github.com/sumup/sumup-go"
	"github.com/sumup/sumup-go/client"
)

// Charger implements billing.Charger against SumUp's Checkouts API.
type Charger struct {
	checkouts    *sumup.CheckoutsClient
	merchantCode string
}

// New builds a Charger from a SumUp API key and the merchant code that should
// receive the payments.
func New(apiKey, merchantCode string) *Charger {
	return &Charger{
		checkouts:    sumup.NewCheckoutsClient(client.New(client.WithAPIKey(apiKey))),
		merchantCode: merchantCode,
	}
}

// NewWithClient lets you supply a pre-built SumUp client (e.g. with OAuth tokens
// or a custom http.Client) instead of a bare API key.
func NewWithClient(c *client.Client, merchantCode string) *Charger {
	return &Charger{checkouts: sumup.NewCheckoutsClient(c), merchantCode: merchantCode}
}

// Charge creates a checkout and processes it with the saved card token. It maps
// a synchronously-PAID checkout to a successful billing.ChargeResult; anything
// else (accepted/pending/failed) is reported as not successful so the Engine
// won't advance the billing date.
func (c *Charger) Charge(ctx context.Context, req billing.ChargeRequest) (billing.ChargeResult, error) {
	create := sumup.CheckoutCreateRequest{
		Amount:            float32(req.AmountCents) / 100, // SDK takes major units
		CheckoutReference: req.Reference,
		Currency:          sumup.Currency(req.Currency),
		CustomerID:        &req.CustomerID,
		MerchantCode:      c.merchantCode,
	}
	if req.Description != "" {
		create.Description = &req.Description
	}

	checkout, err := c.checkouts.Create(ctx, create)
	if err != nil {
		return billing.ChargeResult{}, fmt.Errorf("sumupgo: create checkout: %w", err)
	}
	if checkout.ID == nil {
		return billing.ChargeResult{}, fmt.Errorf("sumupgo: create checkout: no id returned")
	}

	resp, err := c.checkouts.Process(ctx, *checkout.ID, sumup.ProcessCheckout{
		PaymentType: sumup.ProcessCheckoutPaymentTypeCard,
		Token:       &req.CardToken,
		CustomerID:  &req.CustomerID,
	})
	if err != nil {
		return billing.ChargeResult{}, fmt.Errorf("sumupgo: process checkout: %w", err)
	}

	if success, ok := resp.AsCheckoutSuccess(); ok &&
		success.Status != nil && *success.Status == sumup.CheckoutSuccessStatusPaid {
		var txID string
		if success.TransactionID != nil {
			txID = *success.TransactionID
		}
		return billing.ChargeResult{TransactionID: txID, Status: "SUCCESSFUL"}, nil
	}

	return billing.ChargeResult{Status: "FAILED"}, nil
}

// Ensure Charger satisfies the interface at compile time.
var _ billing.Charger = (*Charger)(nil)
