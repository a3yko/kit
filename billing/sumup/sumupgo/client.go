package sumupgo

import (
	"context"
	"fmt"

	sumup "github.com/sumup/sumup-go"
	"github.com/sumup/sumup-go/client"
)

// Client is a higher-level wrapper over the SumUp SDK for the saved-card billing
// flow: create a customer, open a checkout the card widget / hosted page can
// complete, then read back the tokenized card to charge later (via Charger).
//
// It complements the recurring Engine in the parent package: the Engine charges
// saved cards on a schedule; this Client is the front-half — getting a card on
// file in the first place — plus card management (list/delete).
type Client struct {
	checkouts    *sumup.CheckoutsClient
	customers    *sumup.CustomersClient
	merchantCode string
}

// NewClient builds a Client from a SumUp API key and the merchant code that
// should receive payments.
func NewClient(apiKey, merchantCode string) *Client {
	c := client.New(client.WithAPIKey(apiKey))
	return &Client{
		checkouts:    sumup.NewCheckoutsClient(c),
		customers:    sumup.NewCustomersClient(c),
		merchantCode: merchantCode,
	}
}

// NewClientWith lets you supply a pre-built SDK client (custom http.Client,
// OAuth tokens, etc.).
func NewClientWith(c *client.Client, merchantCode string) *Client {
	return &Client{
		checkouts:    sumup.NewCheckoutsClient(c),
		customers:    sumup.NewCustomersClient(c),
		merchantCode: merchantCode,
	}
}

// EnsureCustomer creates (or updates) a SumUp customer under your own
// merchant-scoped customerID — the GA way to attach a saved card to a user. Call
// it before CreateSetupCheckout so the tokenized card is linked to this customer
// and reusable for future charges. SumUp treats Create as an upsert on
// customer_id, so this is safe to call repeatedly.
func (c *Client) EnsureCustomer(ctx context.Context, customerID, email, firstName, lastName string) error {
	body := sumup.Customer{
		CustomerID: customerID,
		PersonalDetails: &sumup.PersonalDetails{
			Email:     strptr(email),
			FirstName: strptr(firstName),
			LastName:  strptr(lastName),
		},
	}
	if _, err := c.customers.Create(ctx, body); err != nil {
		return fmt.Errorf("sumupgo: ensure customer %q: %w", customerID, err)
	}
	return nil
}

// Checkout is the result of creating a checkout: an ID for the card widget to
// mount on, and (when hosted) a SumUp-hosted page URL to redirect to.
type Checkout struct {
	ID        string
	HostedURL string // non-empty when Hosted Checkout was requested
}

// CheckoutParams configures a checkout. AmountCents may be 0 for a pure
// card-on-file setup where SumUp permits it; otherwise it's the amount to
// authorize/charge. Set Hosted to receive a SumUp-hosted payment page URL
// instead of mounting the embedded widget.
type CheckoutParams struct {
	CustomerID  string // your merchant-scoped customer id (required to save a card)
	Reference   string // your order/idempotency reference
	AmountCents int
	Currency    string // ISO 4217, e.g. "EUR"
	Description string
	ReturnURL   string // backend callback SumUp notifies on processing updates
	RedirectURL string // where the payer returns after 3DS/redirect
	Hosted      bool   // request a SumUp-hosted payment page
}

// CreateCheckout opens a standard payment checkout (Purpose=CHECKOUT).
func (c *Client) CreateCheckout(ctx context.Context, p CheckoutParams) (Checkout, error) {
	return c.create(ctx, p, sumup.CheckoutCreateRequestPurposeCheckout)
}

// CreateSetupCheckout opens a checkout that saves the card for future
// merchant-initiated charges (Purpose=SETUP_RECURRING_PAYMENT). CustomerID is
// required; the resulting token is read back via GetCheckout once the payer
// completes the widget/hosted page.
func (c *Client) CreateSetupCheckout(ctx context.Context, p CheckoutParams) (Checkout, error) {
	if p.CustomerID == "" {
		return Checkout{}, fmt.Errorf("sumupgo: setup checkout requires a CustomerID")
	}
	return c.create(ctx, p, sumup.CheckoutCreateRequestPurposeSetupRecurringPayment)
}

func (c *Client) create(ctx context.Context, p CheckoutParams, purpose sumup.CheckoutCreateRequestPurpose) (Checkout, error) {
	req := sumup.CheckoutCreateRequest{
		Amount:            float32(p.AmountCents) / 100, // SDK takes major units
		CheckoutReference: p.Reference,
		Currency:          sumup.Currency(p.Currency),
		MerchantCode:      c.merchantCode,
		Purpose:           &purpose,
	}
	if p.CustomerID != "" {
		req.CustomerID = &p.CustomerID
	}
	if p.Description != "" {
		req.Description = &p.Description
	}
	if p.ReturnURL != "" {
		req.ReturnURL = &p.ReturnURL
	}
	if p.RedirectURL != "" {
		req.RedirectURL = &p.RedirectURL
	}
	if p.Hosted {
		req.HostedCheckout = &sumup.HostedCheckout{Enabled: true}
	}

	checkout, err := c.checkouts.Create(ctx, req)
	if err != nil {
		return Checkout{}, fmt.Errorf("sumupgo: create checkout: %w", err)
	}
	out := Checkout{}
	if checkout.ID != nil {
		out.ID = *checkout.ID
	}
	if out.ID == "" {
		return Checkout{}, fmt.Errorf("sumupgo: create checkout: no id returned")
	}
	return out, nil
}

// CheckoutResult is the settled state of a checkout, read after the payer
// completes it. CardToken is the saved payment-instrument token (present once a
// SETUP_RECURRING_PAYMENT checkout is paid); store it to charge the card later.
type CheckoutResult struct {
	ID            string
	Status        string // e.g. "PAID", "PENDING", "FAILED", "EXPIRED"
	Paid          bool
	TransactionID string
	CustomerID    string
	Reference     string
	HostedURL     string
	CardToken     string
}

// GetCheckout fetches a checkout's current state, including the saved card token
// once available.
func (c *Client) GetCheckout(ctx context.Context, id string) (CheckoutResult, error) {
	ck, err := c.checkouts.Get(ctx, id)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("sumupgo: get checkout %q: %w", id, err)
	}
	res := CheckoutResult{ID: id}
	if ck.Status != nil {
		res.Status = string(*ck.Status)
		res.Paid = *ck.Status == sumup.CheckoutSuccessStatusPaid
	}
	if ck.TransactionID != nil {
		res.TransactionID = *ck.TransactionID
	}
	if ck.CustomerID != nil {
		res.CustomerID = *ck.CustomerID
	}
	if ck.CheckoutReference != nil {
		res.Reference = *ck.CheckoutReference
	}
	if ck.HostedCheckoutURL != nil {
		res.HostedURL = *ck.HostedCheckoutURL
	}
	if ck.PaymentInstrument != nil && ck.PaymentInstrument.Token != nil {
		res.CardToken = *ck.PaymentInstrument.Token
	}
	return res, nil
}

// SavedCard is a tokenized card on file for a customer.
type SavedCard struct {
	Token  string
	Last4  string
	Brand  string
	Active bool
}

// ListSavedCards returns the active and inactive cards saved for a customer.
func (c *Client) ListSavedCards(ctx context.Context, customerID string) ([]SavedCard, error) {
	resp, err := c.customers.ListPaymentInstruments(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("sumupgo: list cards for %q: %w", customerID, err)
	}
	if resp == nil {
		return nil, nil
	}
	cards := make([]SavedCard, 0, len(*resp))
	for _, pi := range *resp {
		card := SavedCard{Active: pi.Active == nil || *pi.Active}
		if pi.Token != nil {
			card.Token = *pi.Token
		}
		if pi.Card != nil {
			if pi.Card.Last4Digits != nil {
				card.Last4 = *pi.Card.Last4Digits
			}
			if pi.Card.Type != nil {
				card.Brand = string(*pi.Card.Type)
			}
		}
		cards = append(cards, card)
	}
	return cards, nil
}

// DeleteSavedCard deactivates a saved card by its token.
func (c *Client) DeleteSavedCard(ctx context.Context, customerID, token string) error {
	if err := c.customers.DeactivatePaymentInstrument(ctx, customerID, token); err != nil {
		return fmt.Errorf("sumupgo: delete card %q: %w", token, err)
	}
	return nil
}

func strptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
