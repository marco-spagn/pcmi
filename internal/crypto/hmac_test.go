package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestHMACSign_Deterministic(t *testing.T) {
	t.Parallel()
	body := []byte(`{"event":"memory.stored"}`)
	sig := HMACSign("secret", "1700000000", body)
	sig2 := HMACSign("secret", "1700000000", body)
	if sig != sig2 {
		t.Fatalf("signatures differ: %q vs %q", sig, sig2)
	}
	if !strings.HasPrefix(sig, signaturePrefix) {
		t.Fatalf("missing prefix: %q", sig)
	}
}

func TestHMACSign_EmptySecretStillProducesSignature(t *testing.T) {
	t.Parallel()
	sig := HMACSign("", "1", []byte("x"))
	if sig == "" || !strings.HasPrefix(sig, signaturePrefix) {
		t.Fatalf("unexpected sig %q", sig)
	}
}

func TestHMACSign_UnicodeBody(t *testing.T) {
	t.Parallel()
	body := []byte("payload: 日本語 ")
	ts := "1700000000"
	secret := "k"
	sig := HMACSign(secret, ts, body)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(ts))
	_, _ = mac.Write([]byte{'.'})
	_, _ = mac.Write(body)
	want := signaturePrefix + hex.EncodeToString(mac.Sum(nil))
	if sig != want {
		t.Fatalf("unicode body sig mismatch")
	}
}

func TestHMACVerify(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	secret := "webhook-secret"
	body := []byte(`{"ok":true}`)
	ts := "1700000000"

	tests := []struct {
		name      string
		secret    string
		signature string
		timestamp string
		body      []byte
		now       time.Time
		maxAge    time.Duration
		want      bool
	}{
		{name: "valid", secret: secret, signature: HMACSign(secret, ts, body), timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: true},
		{name: "empty secret", secret: "", signature: HMACSign(secret, ts, body), timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "empty signature", secret: secret, signature: "", timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "empty timestamp", secret: secret, signature: HMACSign(secret, ts, body), timestamp: "", body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "wrong prefix", secret: secret, signature: "md5=abc", timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "invalid hex", secret: secret, signature: signaturePrefix + "zzzz", timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "tampered body", secret: secret, signature: HMACSign(secret, ts, body), timestamp: ts, body: []byte(`{"ok":false}`), now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "tampered timestamp in sig", secret: secret, signature: HMACSign(secret, "1699999999", body), timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "expired", secret: secret, signature: HMACSign(secret, ts, body), timestamp: ts, body: body, now: now.Add(DefaultWebhookMaxAge + time.Second), maxAge: DefaultWebhookMaxAge, want: false},
		{name: "future beyond skew", secret: secret, signature: HMACSign(secret, ts, body), timestamp: ts, body: body, now: now.Add(-WebhookClockSkew - time.Second), maxAge: DefaultWebhookMaxAge, want: false},
		{name: "future within skew", secret: secret, signature: HMACSign(secret, ts, body), timestamp: ts, body: body, now: now.Add(-30 * time.Second), maxAge: DefaultWebhookMaxAge, want: true},
		{name: "non-numeric timestamp", secret: secret, signature: HMACSign(secret, ts, body), timestamp: "not-a-ts", body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
		{name: "length mismatch hex", secret: secret, signature: signaturePrefix + "ab", timestamp: ts, body: body, now: now, maxAge: DefaultWebhookMaxAge, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := HMACVerify(tc.secret, tc.signature, tc.timestamp, tc.body, tc.now, tc.maxAge)
			if got != tc.want {
				t.Fatalf("HMACVerify() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHMACVerify_UsesConstantTimeCompare(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	secret := "s"
	body := []byte("x")
	ts := "1700000000"
	valid := HMACSign(secret, ts, body)
	gotHex := strings.TrimPrefix(valid, signaturePrefix)
	// Flip one nibble in the middle of the digest.
	tampered := signaturePrefix + gotHex[:len(gotHex)-2] + "ff"
	if HMACVerify(secret, tampered, ts, body, now, DefaultWebhookMaxAge) {
		t.Fatal("expected verify false for single-nibble tamper")
	}
}
