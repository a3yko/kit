package storage

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Presigning is done locally (no network), so we can verify URL construction
// with dummy credentials.
func TestPresignGet(t *testing.T) {
	b := NewR2("acct123", "AKIDEXAMPLE", "secretexample", "documents")

	url, err := b.PresignGet(context.Background(), "vehicles/abc/title.pdf", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}

	for _, want := range []string{
		"https://",
		"acct123.r2.cloudflarestorage.com",
		"documents",              // bucket (virtual-hosted host or path)
		"vehicles/abc/title.pdf", // the object key
		"X-Amz-Signature=",       // it is actually signed
		"X-Amz-Expires=900",      // 15m TTL
	} {
		if !strings.Contains(url, want) {
			t.Errorf("presigned URL missing %q:\n%s", want, url)
		}
	}
}

func TestPresignPutSetsExpiry(t *testing.T) {
	b := NewR2("acct123", "AKIDEXAMPLE", "secretexample", "documents")
	url, err := b.PresignPut(context.Background(), "k", "application/pdf", time.Hour)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if !strings.Contains(url, "X-Amz-Expires=3600") {
		t.Errorf("expected 1h expiry in URL:\n%s", url)
	}
}
