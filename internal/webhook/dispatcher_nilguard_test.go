package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestDispatcher_Close verifies the stop channel is closed cleanly and the
// background retry goroutine exits without panic even when db is nil.
func TestDispatcher_Close(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(nil, 3)
	done := make(chan struct{})
	go func() {
		d.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5s")
	}
}

// TestDispatcher_NotifyMatching_NilGuards exercises all early-return branches
// in NotifyMatching that do not require a live database.
func TestDispatcher_NotifyMatching_NilGuards(t *testing.T) {
	t.Parallel()

	// nil dispatcher receiver
	var nilD *Dispatcher
	nilD.NotifyMatching("tenant", "event", nil) // must not panic

	// nil db
	d := &Dispatcher{}
	d.NotifyMatching("tenant", "event", nil) // db == nil → early return

	// empty tenantID
	d2 := &Dispatcher{}
	d2.NotifyMatching("", "event", nil) // tenantID == "" → early return
}

// TestDispatcher_RefreshPendingOldestAge_NilDB verifies the guard that avoids a
// nil-pointer dereference when the database connection is absent.
func TestDispatcher_RefreshPendingOldestAge_NilDB(t *testing.T) {
	t.Parallel()
	d := &Dispatcher{}
	ctx := context.Background()
	d.refreshPendingOldestAge(ctx) // must not panic; sets metric to 0

	var nilD *Dispatcher
	nilD.refreshPendingOldestAge(ctx) // nil receiver guard
}

// TestDispatcher_Post_NilClientFallback confirms that passing a nil *http.Client
// to post() falls through to http.DefaultClient and succeeds.
func TestDispatcher_Post_NilClientFallback(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := &Dispatcher{client: nil}
	if err := d.post(context.Background(), srv.URL, "", []byte("{}")); err != nil {
		t.Fatalf("post with nil client: %v", err)
	}
}
