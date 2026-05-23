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

func TestNewDelivery_DefaultTimestamp(t *testing.T) {
	t.Parallel()
	before := time.Now().UTC().Unix()
	d := NewDelivery("d1", "https://example.com", "sec", []byte("{}"), 0)
	after := time.Now().UTC().Unix()
	if d.Timestamp < before || d.Timestamp > after {
		t.Fatalf("timestamp %d not in [%d,%d]", d.Timestamp, before, after)
	}
}

func TestDelivery_ApplyHeaders_WithSecret(t *testing.T) {
	t.Parallel()
	ts := int64(1_700_000_000)
	body := []byte(`{"event":"x"}`)
	d := NewDelivery("del-1", "https://x", "secret", body, ts)

	req, err := http.NewRequest(http.MethodPost, d.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.ApplyHeaders(req)

	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("X-PCMI-Event-Delivery") != "1" {
		t.Fatal("missing X-PCMI-Event-Delivery")
	}
	if req.Header.Get("X-PCMI-Delivery-ID") != "del-1" {
		t.Fatalf("delivery id = %q", req.Header.Get("X-PCMI-Delivery-ID"))
	}
	tsStr := strconv.FormatInt(ts, 10)
	if req.Header.Get("X-PCMI-Timestamp") != tsStr {
		t.Fatalf("timestamp header = %q want %q", req.Header.Get("X-PCMI-Timestamp"), tsStr)
	}
	sig := req.Header.Get("X-PCMI-Signature")
	now := time.Unix(ts, 0)
	if !crypto.HMACVerify("secret", sig, tsStr, body, now, crypto.DefaultWebhookMaxAge) {
		t.Fatalf("signature not verifiable: %q", sig)
	}
}

func TestDelivery_ApplyHeaders_NoSecretOmitsSignature(t *testing.T) {
	t.Parallel()
	d := NewDelivery("id", "https://x", "", []byte("{}"), 1)
	req, _ := http.NewRequest(http.MethodPost, d.URL, nil)
	d.ApplyHeaders(req)
	if req.Header.Get("X-PCMI-Signature") != "" {
		t.Fatal("expected no signature without secret")
	}
}

func TestDelivery_Post_SuccessAndFailure(t *testing.T) {
	t.Parallel()
	body := []byte(`{"k":1}`)
	secret := "hook-secret"
	ts := time.Now().UTC().Unix()

	t.Run("2xx", func(t *testing.T) {
		t.Parallel()
		var gotTS, gotSig string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotTS = r.Header.Get("X-PCMI-Timestamp")
			gotSig = r.Header.Get("X-PCMI-Signature")
			b, _ := io.ReadAll(r.Body)
			if string(b) != string(body) {
				t.Errorf("body = %q", b)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)

		d := NewDelivery("d", srv.URL, secret, body, ts)
		if err := d.Post(context.Background(), srv.Client()); err != nil {
			t.Fatal(err)
		}
		if gotTS != strconv.FormatInt(ts, 10) {
			t.Fatalf("timestamp header %q", gotTS)
		}
		if !crypto.HMACVerify(secret, gotSig, gotTS, body, time.Unix(ts, 0), crypto.DefaultWebhookMaxAge) {
			t.Fatal("receiver could not verify signature")
		}
	})

	t.Run("5xx", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(srv.Close)

		d := NewDelivery("d", srv.URL, secret, body, ts)
		err := d.Post(context.Background(), srv.Client())
		if err == nil {
			t.Fatal("expected error for 502")
		}
	})
}

func TestDelivery_Post_ExpiredTimestampRejectedByReceiver(t *testing.T) {
	t.Parallel()
	oldTS := time.Now().UTC().Add(-2 * crypto.DefaultWebhookMaxAge).Unix()
	body := []byte("{}")
	secret := "s"
	d := NewDelivery("d", "https://example.com", secret, body, oldTS)
	tsStr := d.timestampString()
	sig := crypto.HMACSign(secret, tsStr, body)
	now := time.Now().UTC()
	if crypto.HMACVerify(secret, sig, tsStr, body, now, crypto.DefaultWebhookMaxAge) {
		t.Fatal("stale delivery timestamp should fail verify at receiver")
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
