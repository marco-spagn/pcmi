package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
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

	t.Run("hmac signature", func(t *testing.T) {
		secret := "test-secret"
		body := []byte(`{"event_type":"x"}`)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-PCMI-Signature"); got != want {
				t.Errorf("signature = %q, want %q", got, want)
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
