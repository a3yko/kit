package sumup

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func TestVerifyWebhook(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"event_type":"CHECKOUT_STATUS_CHANGED","id":"abc"}`)
	good := sign(secret, body)

	cases := []struct {
		name   string
		secret string
		body   []byte
		sig    string
		want   bool
	}{
		{"valid", secret, body, good, true},
		{"valid with prefix", secret, body, "sha256=" + good, true},
		{"valid uppercase", secret, body, strings.ToUpper(good), true},
		{"tampered body", secret, []byte(`{"event_type":"x"}`), good, false},
		{"wrong secret", "other", body, good, false},
		{"empty secret", "", body, good, false},
		{"empty sig", secret, body, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyWebhook(tc.secret, tc.body, tc.sig); got != tc.want {
				t.Errorf("VerifyWebhook = %v, want %v", got, tc.want)
			}
		})
	}
}
