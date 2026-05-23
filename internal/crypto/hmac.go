package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const signaturePrefix = "sha256="

// DefaultWebhookMaxAge is the maximum age accepted for X-PCMI-Timestamp on webhook verify.
const DefaultWebhookMaxAge = 5 * time.Minute

// WebhookClockSkew allows receivers' clocks slightly ahead of the signer.
const WebhookClockSkew = 1 * time.Minute

// HMACSign returns the X-PCMI-Signature value: sha256={hex(HMAC-SHA256(secret, timestamp+"."+body))}.
func HMACSign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte{'.'})
	_, _ = mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// HMACVerify checks a webhook signature and timestamp freshness.
func HMACVerify(secret, signature, timestamp string, body []byte, now time.Time, maxAge time.Duration) bool {
	if secret == "" || signature == "" || timestamp == "" {
		return false
	}
	if !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}
	gotHex := strings.TrimPrefix(signature, signaturePrefix)
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	eventTime := time.Unix(ts, 0)
	if now.Sub(eventTime) > maxAge || eventTime.Sub(now) > WebhookClockSkew {
		return false
	}
	wantHex := strings.TrimPrefix(HMACSign(secret, timestamp, body), signaturePrefix)
	want, err := hex.DecodeString(wantHex)
	if err != nil {
		return false
	}
	return hmac.Equal(got, want)
}
