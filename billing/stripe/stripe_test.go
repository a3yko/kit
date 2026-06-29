package stripe

import "testing"

func TestNew(t *testing.T) {
	if c := New("sk_test_x", "whsec_x"); c == nil {
		t.Fatal("New returned nil")
	}
}

func TestVerifyWebhookRejectsBadSignature(t *testing.T) {
	c := New("sk_test_x", "whsec_secret")
	// A payload with a missing/garbage Stripe-Signature must not verify.
	if _, err := c.VerifyWebhook([]byte(`{"id":"evt_1"}`), "t=1,v1=deadbeef"); err == nil {
		t.Fatal("VerifyWebhook accepted an invalid signature")
	}
}
