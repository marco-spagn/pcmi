package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/crypto"
)

func TestDispatcherPost(t *testing.T) {
	t.Run("headers and 2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-PCMI-Event-Delivery") != "1" {
				t.Error("expected X-PCMI-Event-Delivery header")
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		d := &Dispatcher{client: srv.Client()}
		err := d.post(context.Background(), srv.URL, "", []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("hmac signature with timestamp", func(t *testing.T) {
		secret := "test-secret"
		body := []byte(`{"event_type":"x"}`)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ts := r.Header.Get("X-PCMI-Timestamp")
			if ts == "" {
				t.Error("missing X-PCMI-Timestamp")
			}
			payload, _ := io.ReadAll(r.Body)
			if string(payload) != string(body) {
				t.Errorf("body = %q", payload)
			}
			sig := r.Header.Get("X-PCMI-Signature")
			tsInt, err := strconv.ParseInt(ts, 10, 64)
			if err != nil {
				t.Fatalf("timestamp: %v", err)
			}
			if !crypto.HMACVerify(secret, sig, ts, payload, time.Unix(tsInt, 0), crypto.DefaultWebhookMaxAge) {
				t.Errorf("signature verify failed: sig=%q ts=%q", sig, ts)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		d := &Dispatcher{client: srv.Client()}
		if err := d.post(context.Background(), srv.URL, secret, body); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("non-2xx status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		d := &Dispatcher{client: srv.Client()}
		err := d.post(context.Background(), srv.URL, "", []byte("{}"))
		if err == nil {
			t.Fatal("expected error for 503")
		}
	})
}

func TestNotifyMatchingEarlyReturn(t *testing.T) {
	var nilD *Dispatcher
	nilD.NotifyMatching("tenant", "memory.stored", nil)

	d := &Dispatcher{}
	d.NotifyMatching("", "memory.stored", nil)
	d.NotifyMatching("00000000-0000-0000-0000-000000000000", "memory.stored", nil)
}
