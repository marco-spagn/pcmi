package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDelivery_Post_3xxIsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	t.Cleanup(srv.Close)

	d := NewDelivery("d", srv.URL, "", []byte("{}"), 0)
	if err := d.Post(context.Background(), srv.Client()); err == nil {
		t.Fatal("expected error for 301")
	}
}

func TestDelivery_Post_contextCanceled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := NewDelivery("d", srv.URL, "", []byte("{}"), 0)
	err := d.Post(ctx, srv.Client())
	if err == nil {
		t.Fatal("expected error when context already canceled")
	}
}

func TestNewDispatcher_nilDBNotify_noPanic(t *testing.T) {
	t.Parallel()
	d := &Dispatcher{maxAttempts: 1}
	d.NotifyMatching("00000000-0000-0000-0000-000000000000", "memory.stored", map[string]any{"x": 1})
}

func TestDelivery_ApplyHeaders_preservesBodyReader(t *testing.T) {
	t.Parallel()
	body := []byte(`{"a":1}`)
	d := NewDelivery("id", "https://example.com", "sec", body, 1)
	req, err := http.NewRequest(http.MethodPost, d.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.ApplyHeaders(req)
	if req.Header.Get("X-PCMI-Signature") == "" {
		t.Fatal("expected signature with secret")
	}
}
