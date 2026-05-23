package pcmi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Store_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memories" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("api key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["path"] != "root.demo" || body["content"] != "hello" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(StoreResponse{ID: 42, Path: "root.demo", Version: 1})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.Store(context.Background(), "root.demo", "hello", nil, &StoreOptions{Tags: []string{"sdk"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 42 || resp.Path != "root.demo" || resp.Version != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestClient_Store_NetworkError_Retries(t *testing.T) {
	var roundTrips atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(StoreResponse{ID: 7, Path: "root.retry", Version: 1})
	}))
	defer srv.Close()

	transport := &flakyTransport{
		attempts:  &roundTrips,
		failUntil: 2,
		base:      http.DefaultTransport,
	}
	c, err := NewClient(srv.URL, "test-key",
		WithHTTPClient(&http.Client{Transport: transport, Timeout: 5 * time.Second}),
		WithRetries(3),
		WithRetryBackoff(5*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.Store(context.Background(), "root.retry", "payload", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 7 {
		t.Fatalf("got id %d, want 7", resp.ID)
	}
	if roundTrips.Load() < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", roundTrips.Load())
	}
}

type flakyTransport struct {
	attempts  *atomic.Int32
	failUntil int32
	base      http.RoundTripper
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	n := f.attempts.Add(1)
	if n <= f.failUntil {
		return nil, &temporaryNetErr{msg: "connection reset by peer"}
	}
	if f.base == nil {
		f.base = http.DefaultTransport
	}
	return f.base.RoundTrip(req)
}

type temporaryNetErr struct {
	msg string
}

func (e *temporaryNetErr) Error() string   { return e.msg }
func (e *temporaryNetErr) Timeout() bool   { return false }
func (e *temporaryNetErr) Temporary() bool { return true }

func TestClient_Retrieve_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/retrieve" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(RetrieveResponse{
			Total: 1,
			Entries: []MemoryEntry{{
				ID:      99,
				Path:    "root.find",
				Content: "found",
				Tags:    []string{"a"},
			}},
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	out, err := c.Retrieve(context.Background(), "root", "query", 5, &RetrieveOptions{TagsMatch: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || len(out.Entries) != 1 {
		t.Fatalf("unexpected response: %+v", out)
	}
	if out.Entries[0].Path != "root.find" || out.Entries[0].Content != "found" {
		t.Fatalf("unexpected entry: %+v", out.Entries[0])
	}
}

func TestClient_Subscribe_ReceivesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("flush unsupported")
		}
		payload := `{"type":"memory.stored","payload":{"path":"root.sse"}}`
		fmt.Fprintf(w, "event: memory.stored\ndata: %s\n\n", payload)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := c.Subscribe(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		if ev.Type != "memory.stored" {
			t.Fatalf("event type = %q", ev.Type)
		}
		path, _ := ev.Payload["path"].(string)
		if path != "root.sse" {
			t.Fatalf("payload path = %q", path)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestClient_Subscribe_ContextCancellation_Stops(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("flush unsupported")
		}
		fmt.Fprintf(w, ": keepalive\n\n")
		flusher.Flush()
		<-block
		<-r.Context().Done()
	}))
	defer srv.Close()
	defer close(block)

	c, err := NewClient(srv.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Subscribe(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscribe shutdown")
	}
}

func TestClient_RespectTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		io.WriteString(w, `{"id":1,"path":"slow","version":1}`)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "test-key", WithTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Store(context.Background(), "slow", "x", nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout/deadline error, got: %v", err)
	}
}
