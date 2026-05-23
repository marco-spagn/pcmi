package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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
			if r.Header.Get("X-PCMI-Delivery-ID") == "" {
				t.Error("expected X-PCMI-Delivery-ID header")
			}
			if r.Header.Get("X-PCMI-Timestamp") == "" {
				t.Error("expected X-PCMI-Timestamp header")
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
		ts := int64(1715000000)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sig := r.Header.Get("X-PCMI-Signature")
			tsHdr := r.Header.Get("X-PCMI-Timestamp")
			if tsHdr != strconv.FormatInt(ts, 10) {
				t.Errorf("timestamp = %q, want %d", tsHdr, ts)
			}
			want := crypto.HMACSign(secret, tsHdr, body)
			if sig != want {
				t.Errorf("signature = %q, want %q", sig, want)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		del := NewDelivery("del-test", srv.URL, secret, body, ts)
		if err := del.Post(context.Background(), srv.Client()); err != nil {
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
