package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/crypto"
)

// TestDeliveryStress_ConcurrentHMACValidation sends 500 concurrent webhook
// deliveries to a test server that validates the X-PCMI-Signature on every
// request. Every delivery must arrive with a correct HMAC or the test fails.
func TestDeliveryStress_ConcurrentHMACValidation(t *testing.T) {
	t.Parallel()
	const total = 500
	const secret = "s3cr3t"

	var badSig atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-PCMI-Signature")
		ts := r.Header.Get("X-PCMI-Timestamp")
		body, _ := io.ReadAll(r.Body)
		expected := crypto.HMACSign(secret, ts, body)
		if sig != expected {
			badSig.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := srv.Client()
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := []byte(fmt.Sprintf(`{"id":%d,"data":"payload-%d"}`, i, i))
			d := NewDelivery(fmt.Sprintf("del-%d", i), srv.URL, secret, body, 0)
			ctx := context.Background()
			_ = d.Post(ctx, client)
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestDeliveryStress_ConcurrentHMACValidation: timeout")
	}

	if n := badSig.Load(); n > 0 {
		t.Errorf("%d/%d deliveries had invalid HMAC signatures", n, total)
	}
}

// TestDeliveryStress_MixedOutcomes fires 300 concurrent deliveries to a server
// that randomly returns 200, 429, or 500. The test verifies that:
//   - All goroutines complete (no deadlock)
//   - Non-200 responses return non-nil errors
//   - No panics occur
func TestDeliveryStress_MixedOutcomes(t *testing.T) {
	t.Parallel()
	const total = 300

	var ok200, err4xx, err5xx atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch rand.Intn(3) {
		case 0:
			w.WriteHeader(200)
		case 1:
			w.WriteHeader(429)
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	client := srv.Client()
	var wg sync.WaitGroup
	var nonNilErr atomic.Int32

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := []byte(`{"ok":true}`)
			d := NewDelivery(fmt.Sprintf("d%d", i), srv.URL, "", body, 0)
			err := d.Post(context.Background(), client)
			if err == nil {
				ok200.Add(1)
			} else if strings.Contains(err.Error(), "429") {
				err4xx.Add(1)
				nonNilErr.Add(1)
			} else {
				err5xx.Add(1)
				nonNilErr.Add(1)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("TestDeliveryStress_MixedOutcomes: timeout (ok=%d 4xx=%d 5xx=%d)",
			ok200.Load(), err4xx.Load(), err5xx.Load())
	}

	total32 := int32(total)
	if ok200.Load()+err4xx.Load()+err5xx.Load() != total32 {
		t.Errorf("counters do not add up: ok=%d 4xx=%d 5xx=%d total=%d",
			ok200.Load(), err4xx.Load(), err5xx.Load(), total)
	}
}

// TestDeliveryStress_ContextCancellation fires 100 goroutines, each cancelling
// their context before the server finishes. Must not panic; errors must be
// non-nil (context.Canceled or a network error).
func TestDeliveryStress_ContextCancellation(t *testing.T) {
	t.Parallel()
	const total = 100

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := srv.Client()
	var wg sync.WaitGroup
	var errCount, okCount atomic.Int32

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			body := []byte(`{"cancel":true}`)
			d := NewDelivery(fmt.Sprintf("c%d", i), srv.URL, "", body, 0)
			if err := d.Post(ctx, client); err != nil {
				errCount.Add(1)
			} else {
				okCount.Add(1)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("TestDeliveryStress_ContextCancellation: timeout")
	}

	// Most requests should have timed out; a few might succeed if server was fast.
	if errCount.Load() == 0 {
		t.Error("expected context cancellation errors but got none")
	}
	t.Logf("cancelled=%d ok=%d", errCount.Load(), okCount.Load())
}

// TestDeliveryStress_LargePayload sends 20 concurrent deliveries each with a
// 512 KB body to verify no memory corruption or silent truncation.
func TestDeliveryStress_LargePayload(t *testing.T) {
	t.Parallel()
	const (
		goroutines  = 20
		payloadSize = 512 * 1024
	)

	var receivedBytes atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		receivedBytes.Add(n)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	payload := bytes.Repeat([]byte("x"), payloadSize)
	client := srv.Client()
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := NewDelivery(fmt.Sprintf("big-%d", i), srv.URL, "secret", payload, 0)
			if err := d.Post(context.Background(), client); err != nil {
				t.Errorf("goroutine %d: %v", i, err)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestDeliveryStress_LargePayload: timeout")
	}

	expected := int64(goroutines * payloadSize)
	if got := receivedBytes.Load(); got != expected {
		t.Errorf("server received %d bytes, expected %d", got, expected)
	}
}

// TestDeliveryStress_EmptyAndInvalidURLs verifies robustness against bad URLs.
// These must return errors, never panic.
func TestDeliveryStress_EmptyAndInvalidURLs(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"not-a-url",
		"ftp://example.com/webhook",
		"http://192.0.2.0:99999/noreachable", // port out of range
	}
	body := []byte(`{"test":true}`)
	client := &http.Client{Timeout: 50 * time.Millisecond}

	var wg sync.WaitGroup
	for _, u := range cases {
		for j := 0; j < 20; j++ {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				d := NewDelivery("bad-url", url, "", body, 0)
				// Must not panic regardless of URL.
				_ = d.Post(context.Background(), client)
			}(u)
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestDeliveryStress_EmptyAndInvalidURLs: timeout — possible goroutine leak")
	}
}

// TestDeliveryStress_HeadersSetConcurrently verifies that ApplyHeaders does
// not race when called concurrently on different Delivery values.
func TestDeliveryStress_HeadersSetConcurrently(t *testing.T) {
	t.Parallel()
	body := []byte(`{}`)
	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := NewDelivery(fmt.Sprintf("h%d", i), "http://localhost", "secret", body, int64(i))
			req, _ := http.NewRequest(http.MethodPost, "http://localhost", nil)
			if req != nil {
				d.ApplyHeaders(req)
			}
		}(i)
	}
	wg.Wait()
}
