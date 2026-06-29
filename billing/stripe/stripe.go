// Package stripe is a thin, decoupled wrapper over the Stripe API for
// subscription billing: start a hosted Checkout Session (cards, 3DS, promo
// codes), open the Customer Portal (change plan/card, cancel, invoices), verify
// webhooks, and look up a checkout session's or a customer's subscription so you
// can sync state immediately instead of waiting on the async webhook.
//
// Like kit/billing/sumup, it is intentionally provider-generic and storage-free:
// it knows nothing about your plans, tenants, or database. Map your own plan keys
// to Stripe price ids and persist subscription state in your own layer — Stripe
// owns renewals and discounts (coupons). See cartradetracker's internal/billing
// for a reference adapter (plan mapping + DB sync + entitlement gate).
package stripe

import (
	"github.com/stripe/stripe-go/v79"
	bpsession "github.com/stripe/stripe-go/v79/billingportal/session"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/customer"
	"github.com/stripe/stripe-go/v79/subscription"
	"github.com/stripe/stripe-go/v79/webhook"
)

// Client wraps the Stripe API for subscription billing. The Stripe secret key is
// configured globally on the SDK by New; webhookSecret is used by VerifyWebhook.
type Client struct {
	webhookSecret string
}

// New configures the global Stripe key and returns a client. webhookSecret is the
// signing secret for the webhook endpoint (used by VerifyWebhook).
func New(secretKey, webhookSecret string) *Client {
	stripe.Key = secretKey
	return &Client{webhookSecret: webhookSecret}
}

// EnsureCustomer returns existing (when non-empty) or creates a Stripe customer
// for the given email, tagging it with metadata (e.g. {"tenant_id": id}) so a
// webhook can be mapped back to your record.
func (c *Client) EnsureCustomer(existing, email string, metadata map[string]string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	cust, err := customer.New(&stripe.CustomerParams{
		Email:  stripe.String(email),
		Params: stripe.Params{Metadata: metadata},
	})
	if err != nil {
		return "", err
	}
	return cust.ID, nil
}

// CheckoutURL starts a subscription Checkout Session for priceID and returns its
// hosted URL. clientRef is stored as the session's client_reference_id (e.g. your
// tenant id). Stripe's "Add promotion code" field is enabled, so discounts are
// just Stripe coupons (duration once/forever/repeating set on the coupon).
func (c *Client) CheckoutURL(customerID, priceID, clientRef, successURL, cancelURL string) (string, error) {
	sess, err := session.New(&stripe.CheckoutSessionParams{
		Mode:                stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:            stripe.String(customerID),
		ClientReferenceID:   stripe.String(clientRef),
		AllowPromotionCodes: stripe.Bool(true),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(priceID), Quantity: stripe.Int64(1)},
		},
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	})
	if err != nil {
		return "", err
	}
	return sess.URL, nil
}

// PortalURL opens the Stripe Customer Portal (change plan/card, cancel, invoices)
// and returns its hosted URL.
func (c *Client) PortalURL(customerID, returnURL string) (string, error) {
	p, err := bpsession.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	})
	if err != nil {
		return "", err
	}
	return p.URL, nil
}

// SessionSubscription retrieves a completed Checkout Session's subscription (with
// its line items + price expanded) so a return handler can sync immediately,
// without waiting on the async webhook. Returns nil if the session has no
// subscription yet.
func (c *Client) SessionSubscription(sessionID string) (*stripe.Subscription, error) {
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("subscription")
	params.AddExpand("subscription.items.data.price")
	sess, err := session.Get(sessionID, params)
	if err != nil {
		return nil, err
	}
	return sess.Subscription, nil
}

// CustomerSubscription returns the customer's most recent subscription (any
// status), for refreshing your copy after the customer changes it in the portal
// (cancel, plan switch, card update) without waiting on the webhook. Returns nil
// if the customer has none.
func (c *Client) CustomerSubscription(customerID string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionListParams{Customer: stripe.String(customerID)}
	params.Status = stripe.String("all")
	params.Limit = stripe.Int64(1) // newest first
	params.AddExpand("data.items.data.price")
	iter := subscription.List(params)
	if iter.Next() {
		return iter.Subscription(), nil
	}
	return nil, iter.Err()
}

// VerifyWebhook validates the Stripe-Signature header against the signing secret
// and returns the parsed event.
func (c *Client) VerifyWebhook(payload []byte, sigHeader string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, sigHeader, c.webhookSecret)
}
