package crypto

import (
	"testing"
	"time"
)

func TestHMACSign_DeterministicForSameInput(t *testing.T) {
	secret := "whsec_test"
	ts := "1715000000"
	body := []byte(`{"event_type":"memory.stored"}`)
	a := HMACSign(secret, ts, body)
	b := HMACSign(secret, ts, body)
	if a != b {
		t.Fatalf("sign not deterministic: %q vs %q", a, b)
	}
	if a[:7] != "sha256=" {
		t.Fatalf("expected sha256= prefix, got %q", a)
	}
}

func TestHMACVerify_ValidSignature_ReturnsTrue(t *testing.T) {
	secret := "whsec_test"
	ts := "1715000000"
	body := []byte(`{"ok":true}`)
	now := time.Unix(1715000060, 0)
	sig := HMACSign(secret, ts, body)
	if !HMACVerify(secret, sig, ts, body, now, DefaultWebhookMaxAge) {
		t.Fatal("expected valid signature")
	}
}

func TestHMACVerify_InvalidSignature_ReturnsFalse(t *testing.T) {
	secret := "whsec_test"
	ts := "1715000000"
	body := []byte(`{"ok":true}`)
	now := time.Unix(1715000060, 0)
	if HMACVerify(secret, "sha256=deadbeef", ts, body, now, DefaultWebhookMaxAge) {
		t.Fatal("expected invalid hex signature to fail")
	}
	if HMACVerify(secret, HMACSign("other-secret", ts, body), ts, body, now, DefaultWebhookMaxAge) {
		t.Fatal("expected wrong secret to fail")
	}
}

func TestHMACVerify_ExpiredTimestamp_ReturnsFalse(t *testing.T) {
	secret := "whsec_test"
	ts := "1715000000"
	body := []byte(`{"ok":true}`)
	now := time.Unix(1715000400, 0) // 400s later
	sig := HMACSign(secret, ts, body)
	if HMACVerify(secret, sig, ts, body, now, DefaultWebhookMaxAge) {
		t.Fatal("expected expired timestamp to fail")
	}
}
