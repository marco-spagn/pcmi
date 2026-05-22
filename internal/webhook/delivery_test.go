package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/crypto"
)

func TestWebhookDelivery_IncludesSignatureHeader(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event_type":"memory.stored"}`)
	ts := time.Now().UTC().Unix()
	del := NewDelivery("del-123", "", secret, body, ts)

	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	del.ApplyHeaders(req)

	if got := req.Header.Get("X-PCMI-Signature"); got == "" {
		t.Fatal("expected X-PCMI-Signature header")
	}
	if got := req.Header.Get("X-PCMI-Timestamp"); got != strconv.FormatInt(ts, 10) {
		t.Errorf("timestamp = %q, want %q", got, strconv.FormatInt(ts, 10))
	}
	if got := req.Header.Get("X-PCMI-Delivery-ID"); got != "del-123" {
		t.Errorf("delivery id = %q, want del-123", got)
	}
}

func TestWebhookDelivery_SignatureIsValidHMAC(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event_type":"x"}`)
	ts := int64(1715000000)
	del := NewDelivery("del-1", "", secret, body, ts)

	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		sig := r.Header.Get("X-PCMI-Signature")
		tsHdr := r.Header.Get("X-PCMI-Timestamp")
		now := time.Unix(ts, 0).Add(30 * time.Second)
		if !crypto.HMACVerify(secret, sig, tsHdr, capturedBody, now, crypto.DefaultWebhookMaxAge) {
			t.Error("signature verification failed")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	del.URL = srv.URL
	if err := del.Post(context.Background(), srv.Client()); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookDelivery_NotLegacyBodyOnlyHMAC(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event_type":"x"}`)
	ts := int64(1715000000)
	del := NewDelivery("del-1", "", secret, body, ts)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	legacy := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	del.ApplyHeaders(req)
	if req.Header.Get("X-PCMI-Signature") == legacy {
		t.Fatal("signature must use timestamp.body scheme, not body-only")
	}
}
