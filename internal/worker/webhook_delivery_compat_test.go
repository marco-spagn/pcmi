package worker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/crypto"
	"github.com/marco-spagn/pcmi/internal/webhook"
)

// TestWorkerPath_WebhookDelivery_VerifiableSignature documents that outbound
// deliveries use timestamped HMAC (same contract workers/consumers expect).
func TestWorkerPath_WebhookDelivery_VerifiableSignature(t *testing.T) {
	t.Parallel()
	secret := "worker-hook-secret"
	body := []byte(`{"event_type":"memory.stored"}`)
	ts := time.Now().UTC().Unix()

	var gotTS, gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTS = r.Header.Get("X-PCMI-Timestamp")
		gotSig = r.Header.Get("X-PCMI-Signature")
		payload, _ := io.ReadAll(r.Body)
		if string(payload) != string(body) {
			t.Errorf("body mismatch")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := webhook.NewDelivery("w1", srv.URL, secret, body, ts)
	if err := d.Post(context.Background(), srv.Client()); err != nil {
		t.Fatal(err)
	}
	tsInt, err := strconv.ParseInt(gotTS, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !crypto.HMACVerify(secret, gotSig, gotTS, body, time.Unix(tsInt, 0), crypto.DefaultWebhookMaxAge) {
		t.Fatal("delivery signature not verifiable at receiver")
	}
}
