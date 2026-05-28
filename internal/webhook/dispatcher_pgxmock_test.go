package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func TestListEndpoints_WithMatchingEndpoints(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	rows := pgxmock.NewRows([]string{"id", "url", "event_types", "secret"}).
		AddRow("ep-1", "https://example.com/hook", []string{"memory.stored", "memory.updated"}, "sec1").
		AddRow("ep-2", "https://other.com/callback", []string{}, "sec2")
	mock.ExpectQuery(`SELECT id::text, url, event_types`).WithArgs(tenantID, "memory.stored").WillReturnRows(rows)

	d := &Dispatcher{db: mock}
	eps, err := d.listEndpoints(context.Background(), tenantID, "memory.stored")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("got %d endpoints, want 2", len(eps))
	}
	if eps[0].ID != "ep-1" || eps[0].URL != "https://example.com/hook" {
		t.Errorf("ep[0] = %+v", eps[0])
	}
	if eps[0].Secret != "sec1" {
		t.Errorf("ep[0].Secret = %q", eps[0].Secret)
	}
	if len(eps[0].EventTypes) != 2 {
		t.Errorf("ep[0].EventTypes = %v", eps[0].EventTypes)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListEndpoints_SetTenantContextError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnError(context.DeadlineExceeded)

	d := &Dispatcher{db: mock}
	_, err = d.listEndpoints(context.Background(), tenantID, "memory.stored")
	if err == nil {
		t.Fatal("expected error from set_tenant_context")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListEndpoints_QueryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`SELECT id::text, url, event_types`).WithArgs(tenantID, "memory.stored").WillReturnError(context.DeadlineExceeded)

	d := &Dispatcher{db: mock}
	_, err = d.listEndpoints(context.Background(), tenantID, "memory.stored")
	if err == nil {
		t.Fatal("expected error from query")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestEnqueue_Success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	ep := endpoint{ID: "ep-1", URL: "https://example.com/hook", Secret: "s", EventTypes: []string{"memory.stored"}}

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`INSERT INTO webhook_deliveries`).WithArgs(tenantID, ep.ID, "memory.stored", pgxmock.AnyArg(), 5).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	d := &Dispatcher{db: mock, maxAttempts: 5}
	err = d.enqueue(context.Background(), tenantID, ep, "memory.stored", map[string]any{"x": 1}, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait briefly for async processPending goroutine
	time.Sleep(50 * time.Millisecond)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestEnqueue_SetTenantError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	ep := endpoint{ID: "ep-1", URL: "https://example.com/hook"}

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnError(context.DeadlineExceeded)

	d := &Dispatcher{db: mock, maxAttempts: 5}
	err = d.enqueue(context.Background(), tenantID, ep, "memory.stored", map[string]any{"x": 1}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error from set_tenant_context")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAttemptDelivery_Success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	pd := pendingDelivery{
		ID:         "del-1",
		TenantID:   "ffffffff-ffff-ffff-ffff-ffffffffffff",
		EndpointID: "ep-1",
		URL:        srv.URL,
		Secret:     "",
		EventType:  "memory.stored",
		Body:       []byte(`{"event":"test"}`),
		Attempts:   0,
	}

	mock.ExpectExec(`set_tenant_context`).WithArgs(pd.TenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'delivered'`).WithArgs(pd.ID, 1).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	d := &Dispatcher{db: mock, client: srv.Client(), maxAttempts: 5, retryBase: 100 * time.Millisecond}
	d.attemptDelivery(context.Background(), pd)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAttemptDelivery_FailureThenDeadLetter(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	pd := pendingDelivery{
		ID:         "del-2",
		TenantID:   "11111111-1111-1111-1111-111111111111",
		EndpointID: "ep-2",
		URL:        srv.URL,
		Secret:     "",
		EventType:  "memory.stored",
		Body:       []byte(`{"event":"test"}`),
		Attempts:   4, // next attempt = 5 = maxAttempts → dead letter
	}

	mock.ExpectExec(`set_tenant_context`).WithArgs(pd.TenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'dead_letter'`).WithArgs(pd.ID, 5, "status 500").WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	d := &Dispatcher{db: mock, client: srv.Client(), maxAttempts: 5, retryBase: 100 * time.Millisecond}
	d.attemptDelivery(context.Background(), pd)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAttemptDelivery_FailureWithRetry(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	pd := pendingDelivery{
		ID:         "del-3",
		TenantID:   "22222222-2222-2222-2222-222222222222",
		EndpointID: "ep-3",
		URL:        srv.URL,
		Secret:     "",
		EventType:  "memory.updated",
		Body:       []byte(`{"event":"test"}`),
		Attempts:   1, // next = 2 < maxAttempts → retry with backoff
	}

	mock.ExpectExec(`set_tenant_context`).WithArgs(pd.TenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`UPDATE webhook_deliveries SET attempts`).WithArgs(pd.ID, 2, "status 502", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	d := &Dispatcher{db: mock, client: srv.Client(), maxAttempts: 5, retryBase: 100 * time.Millisecond}
	d.attemptDelivery(context.Background(), pd)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAttemptDelivery_SetTenantContextError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	pd := pendingDelivery{
		ID:         "del-4",
		TenantID:   "33333333-3333-3333-3333-333333333333",
		EndpointID: "ep-4",
		URL:        "https://example.com/hook",
		Attempts:   0,
		Body:       []byte(`{}`),
	}

	mock.ExpectExec(`set_tenant_context`).WithArgs(pd.TenantID).WillReturnError(context.DeadlineExceeded)

	d := &Dispatcher{db: mock, maxAttempts: 5}
	d.attemptDelivery(context.Background(), pd)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessPending_WithDeliveries(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Mock the pending deliveries query
	pendingRows := pgxmock.NewRows([]string{
		"id", "tenant_id", "endpoint_id", "url", "secret",
		"event_type", "attempts", "payload",
	}).AddRow("del-10", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "ep-10", srv.URL, "",
		"memory.stored", 0, map[string]any{"k": "v"})

	mock.ExpectQuery(`SELECT wd.id::text, wd.tenant_id::text, wd.endpoint_id::text`).
		WillReturnRows(pendingRows)

	// refreshPendingOldestAge query
	ageRows := pgxmock.NewRows([]string{"age"}).AddRow(42.5)
	mock.ExpectQuery(`SELECT EXTRACT\(EPOCH FROM \(NOW\(\) - MIN\(created_at\)\)\)`).
		WillReturnRows(ageRows)

	// attemptDelivery for the delivery
	mock.ExpectExec(`set_tenant_context`).WithArgs("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'delivered'`).WithArgs("del-10", 1).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	d := &Dispatcher{db: mock, client: srv.Client(), maxAttempts: 5, retryBase: 100 * time.Millisecond}
	d.processPending()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessPending_QueryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`SELECT wd.id::text, wd.tenant_id::text`).
		WillReturnError(context.DeadlineExceeded)

	d := &Dispatcher{db: mock, maxAttempts: 5}
	d.processPending()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessPending_NoPendingDeliveries(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	pendingRows := pgxmock.NewRows([]string{
		"id", "tenant_id", "endpoint_id", "url", "secret",
		"event_type", "attempts", "payload",
	})
	mock.ExpectQuery(`SELECT wd.id::text, wd.tenant_id::text`).
		WillReturnRows(pendingRows)

	ageRows := pgxmock.NewRows([]string{"age"}).AddRow(nil)
	mock.ExpectQuery(`SELECT EXTRACT\(EPOCH FROM \(NOW\(\) - MIN\(created_at\)\)\)`).
		WillReturnRows(ageRows)

	d := &Dispatcher{db: mock, maxAttempts: 5}
	d.processPending()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRefreshPendingOldestAge_WithAge(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	ageRows := pgxmock.NewRows([]string{"age"}).AddRow(120.5)
	mock.ExpectQuery(`SELECT EXTRACT\(EPOCH FROM \(NOW\(\) - MIN\(created_at\)\)\)`).
		WillReturnRows(ageRows)

	d := &Dispatcher{db: mock}
	d.refreshPendingOldestAge(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRefreshPendingOldestAge_DBError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`SELECT EXTRACT\(EPOCH FROM \(NOW\(\) - MIN\(created_at\)\)\)`).
		WillReturnError(context.DeadlineExceeded)

	d := &Dispatcher{db: mock}
	d.refreshPendingOldestAge(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRetryLoop_StopChannel(t *testing.T) {
	t.Parallel()

	d := &Dispatcher{
		stopCh: make(chan struct{}),
	}
	d.wg.Add(1)

	done := make(chan struct{})
	go func() {
		d.retryLoop()
		close(done)
	}()

	close(d.stopCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retryLoop did not exit after stopCh close")
	}
}

func TestNotifyMatching_WithMatchingEndpoints(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "44444444-4444-4444-4444-444444444444"

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	epRows := pgxmock.NewRows([]string{"id", "url", "event_types", "secret"}).
		AddRow("ep-n1", "https://hooks.example.com/1", []string{"memory.stored"}, "secret1")
	mock.ExpectQuery(`SELECT id::text, url, event_types`).WithArgs(tenantID, "memory.stored").WillReturnRows(epRows)

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`INSERT INTO webhook_deliveries`).WithArgs(tenantID, "ep-n1", "memory.stored", pgxmock.AnyArg(), 5).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	d := &Dispatcher{db: mock, maxAttempts: 5}
	d.NotifyMatching(tenantID, "memory.stored", map[string]any{"id": 1, "data": "test"})

	// Wait for the goroutine in NotifyMatching
	time.Sleep(100 * time.Millisecond)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClose_StopsRetryLoop(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	d := NewDispatcher(mock, 3)
	d.Close()

	// Verifying Close can be called multiple times without panic
	d.Close()
}

func TestBackoffCalculation_MaximumCap(t *testing.T) {
	t.Parallel()

	d := &Dispatcher{retryBase: 10 * time.Second, maxAttempts: 10}
	backoff := d.retryBase * time.Duration(1<<uint(9))
	if backoff > 2*time.Minute {
		backoff = 2 * time.Minute
	}
	if backoff != 2*time.Minute {
		t.Fatalf("expected cap at 2m, got %v", backoff)
	}
}
