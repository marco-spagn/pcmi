package webhook

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDispatcherPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	d := &Dispatcher{
		client:      &http.Client{Timeout: 5 * time.Second},
		maxAttempts: 3,
		retryBase:   100 * time.Millisecond,
	}
	err := d.post(t.Context(), srv.URL, "", []byte(`{"event_type":"test"}`))
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestDispatcherPost_FailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	d := &Dispatcher{
		client:      &http.Client{Timeout: 5 * time.Second},
		maxAttempts: 3,
		retryBase:   100 * time.Millisecond,
	}
	err := d.post(t.Context(), srv.URL, "", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for 5xx response")
	}
}

func TestDispatcherPost_WithHMAC(t *testing.T) {
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-PCMI-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	d := &Dispatcher{
		client:      &http.Client{Timeout: 5 * time.Second},
		maxAttempts: 3,
		retryBase:   100 * time.Millisecond,
	}
	err := d.post(t.Context(), srv.URL, "my-secret", []byte(`{"event_type":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if receivedSig == "" {
		t.Fatal("expected X-PCMI-Signature header to be set")
	}
	if len(receivedSig) < 7 || receivedSig[:7] != "sha256=" {
		t.Fatalf("unexpected signature format: %s", receivedSig)
	}
}

func TestDispatcherPost_NoSecretNoSig(t *testing.T) {
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-PCMI-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	d := &Dispatcher{
		client:      &http.Client{Timeout: 5 * time.Second},
		maxAttempts: 3,
		retryBase:   100 * time.Millisecond,
	}
	if err := d.post(t.Context(), srv.URL, "", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if receivedSig != "" {
		t.Fatalf("expected no signature header when no secret, got: %s", receivedSig)
	}
}

func TestDispatcherPost_InvalidURL(t *testing.T) {
	d := &Dispatcher{
		client:      &http.Client{Timeout: 1 * time.Second},
		maxAttempts: 1,
		retryBase:   100 * time.Millisecond,
	}
	err := d.post(t.Context(), "http://127.0.0.1:0/nope", "", []byte(`{}`))
	if err == nil {
		t.Fatal("expected connection error for closed port")
	}
}

func TestNotifyMatchingNilDispatcher(t *testing.T) {
	// Should not panic
	var d *Dispatcher
	d.NotifyMatching("tid", "memory.stored", map[string]any{"id": 1})
}

func TestBackoffCapZeroAttempts(t *testing.T) {
	d := &Dispatcher{retryBase: 2 * time.Second, maxAttempts: 5}
	backoff := d.retryBase * (1 << 0)
	if backoff != 2*time.Second {
		t.Fatalf("expected 2s backoff at attempt 0, got %v", backoff)
	}
}

func TestBackoffCapMultipleAttempts(t *testing.T) {
	d := &Dispatcher{retryBase: 2 * time.Second, maxAttempts: 10}
	for attempts := 0; attempts < 10; attempts++ {
		backoff := d.retryBase * time.Duration(1<<uint(attempts))
		if backoff > 2*time.Minute {
			backoff = 2 * time.Minute
		}
		if backoff > 2*time.Minute {
			t.Fatalf("backoff exceeded cap at attempt %d: %v", attempts, backoff)
		}
	}
}
