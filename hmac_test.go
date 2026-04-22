package pingback

import (
	"fmt"
	"testing"
	"time"
)

func TestVerifySignature_Valid(t *testing.T) {
	secret := "test-secret"
	body := `{"function":"cleanup"}`
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := computeHMAC(ts, body, secret)

	err := verifySignature(sig, ts, body, secret)
	if err != nil {
		t.Fatalf("expected valid signature, got error: %v", err)
	}
}

func TestVerifySignature_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	body := `{"function":"cleanup"}`
	ts := fmt.Sprintf("%d", time.Now().Unix())

	err := verifySignature("bad-signature", ts, body, secret)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestVerifySignature_ExpiredTimestamp(t *testing.T) {
	secret := "test-secret"
	body := `{"function":"cleanup"}`
	ts := fmt.Sprintf("%d", time.Now().Add(-6*time.Minute).Unix())
	sig := computeHMAC(ts, body, secret)

	err := verifySignature(sig, ts, body, secret)
	if err == nil {
		t.Fatal("expected error for expired timestamp")
	}
}

func TestVerifySignature_TamperedBody(t *testing.T) {
	secret := "test-secret"
	body := `{"function":"cleanup"}`
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := computeHMAC(ts, body, secret)

	err := verifySignature(sig, ts, `{"function":"malicious"}`, secret)
	if err == nil {
		t.Fatal("expected error for tampered body")
	}
}
