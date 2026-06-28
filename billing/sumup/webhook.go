package sumup

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HeaderSignature is the header SumUp puts the webhook signature in.
const HeaderSignature = "X-Payload-Signature"

// VerifyWebhook reports whether sig authenticates body under secret, using
// HMAC-SHA256 over the raw request body and a constant-time comparison. Pass the
// exact bytes received (do not re-marshal — any whitespace change breaks the
// MAC) and the signature header value.
//
// The comparison tolerates a "sha256=" prefix and is case-insensitive on the hex
// digest. An empty secret or signature returns false.
func VerifyWebhook(secret string, body []byte, sig string) bool {
	if secret == "" || sig == "" {
		return false
	}
	sig = strings.TrimSpace(sig)
	sig = strings.TrimPrefix(sig, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	// hex.EncodeToString is lowercase; normalise the incoming value to match.
	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(want))
}
