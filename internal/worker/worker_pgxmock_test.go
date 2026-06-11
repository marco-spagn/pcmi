package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/model"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func newMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })
	return mock
}

func waitDone(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit in time")
	}
}

func any1() []interface{} { return []interface{}{pgxmock.AnyArg()} }
func any2() []interface{} { return []interface{}{pgxmock.AnyArg(), pgxmock.AnyArg()} }
func any3() []interface{} {
	return []interface{}{pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()}
}
func any4() []interface{} {
	return []interface{}{pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()}
}

// ─── ConsolidationWorker ──────────────────────────────────────────────────────

func TestConsolidationWorker_New(t *testing.T) {
	mock := newMock(t)
	w := NewConsolidationWorker(mock)
	if w == nil || w.db == nil || w.repo == nil {
		t.Fatal("NewConsolidationWorker returned incomplete worker")
	}
}

func TestConsolidationWorker_Start_exitsOnCancel(t *testing.T) {
	mock := newMock(t)
	w := NewConsolidationWorker(mock)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	waitDone(t, done)
}

func TestConsolidationWorker_TriggerForMemory_emptyArgs(t *testing.T) {
	w := &ConsolidationWorker{db: nil}
	// All three short-circuit before any DB call — must not panic.
	w.TriggerForMemory("", "root.a")
	w.TriggerForMemory("tenant-id", "")
	w.TriggerForMemory("", "")
}

func TestConsolidationWorker_TriggerForMemory_async(t *testing.T) {
	// parentPrefix("root") == "root" so the goroutine fires.
	// Mock returns error immediately so the goroutine exits fast.
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).WillReturnError(errors.New("no tenant"))
	w := NewConsolidationWorker(mock)
	w.TriggerForMemory("00000000-0000-0000-0000-000000000000", "root")
	time.Sleep(100 * time.Millisecond)
}

func TestConsolidationWorker_processPending_queryError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id, tenant_id`).WillReturnError(errors.New("db error"))
	w := NewConsolidationWorker(mock)
	w.processPending(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidationWorker_processPending_emptyRows(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id, tenant_id`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tenant_id", "path_prefix"}))
	w := NewConsolidationWorker(mock)
	w.processPending(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidationWorker_processPending_scanError(t *testing.T) {
	mock := newMock(t)
	// Row with wrong column count causes Scan to fail → continue.
	mock.ExpectQuery(`SELECT id, tenant_id`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tenant_id"}).AddRow(int64(1), "bad"))
	w := NewConsolidationWorker(mock)
	w.processPending(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidationWorker_countActiveUnderPrefix_success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_entries`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	w := NewConsolidationWorker(mock)
	n, err := w.countActiveUnderPrefix(context.Background(), "tenant-id", "root")
	if err != nil || n != 5 {
		t.Fatalf("got n=%d err=%v", n, err)
	}
}

func TestConsolidationWorker_countActiveUnderPrefix_error(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_entries`).WithArgs(any2()...).
		WillReturnError(errors.New("db error"))
	w := NewConsolidationWorker(mock)
	_, err := w.countActiveUnderPrefix(context.Background(), "tenant-id", "root")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConsolidationWorker_tryConsolidate_countError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_entries`).WithArgs(any2()...).
		WillReturnError(errors.New("db error"))
	w := NewConsolidationWorker(mock)
	err := w.tryConsolidate(context.Background(), "tenant-id", "root")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConsolidationWorker_tryConsolidate_countBelowMin(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_entries`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	w := NewConsolidationWorker(mock)
	if err := w.tryConsolidate(context.Background(), "tenant-id", "root"); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidationWorker_tryConsolidate_pendingExists(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_entries`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM consolidation_runs`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	w := NewConsolidationWorker(mock)
	if err := w.tryConsolidate(context.Background(), "tenant-id", "root"); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidationWorker_tryConsolidate_insert(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_entries`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM consolidation_runs`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`INSERT INTO consolidation_runs`).WithArgs(any3()...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	w := NewConsolidationWorker(mock)
	if err := w.tryConsolidate(context.Background(), "tenant-id", "root"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── EmbeddingWorker ─────────────────────────────────────────────────────────

func TestEmbeddingWorker_Start_exitsOnCancel(t *testing.T) {
	mock := newMock(t)
	w := NewEmbeddingWorker(mock, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	waitDone(t, done)
}

func TestEmbeddingWorker_processPending_bothQueriesError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`list_pending_embeddings`).WillReturnError(errors.New("fn missing"))
	mock.ExpectQuery(`FROM memory_entries`).WillReturnError(errors.New("db error"))
	w := NewEmbeddingWorker(mock, nil)
	w.processPendingEmbeddings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddingWorker_processPending_emptyRows(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`list_pending_embeddings`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "tenant_id"}))
	w := NewEmbeddingWorker(mock, nil)
	w.processPendingEmbeddings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type failProvider struct{}

func (p *failProvider) Generate(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("provider error")
}

func TestEmbeddingWorker_processPending_providerError(t *testing.T) {
	mock := newMock(t)
	tenantID := "00000000-0000-0000-0000-000000000000"
	mock.ExpectQuery(`list_pending_embeddings`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "tenant_id"}).
			AddRow(int64(1), "hello", tenantID))
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	w := NewEmbeddingWorker(mock, &failProvider{})
	w.processPendingEmbeddings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddingWorker_processPending_setTenantError(t *testing.T) {
	mock := newMock(t)
	tenantID := "00000000-0000-0000-0000-000000000000"
	mock.ExpectQuery(`list_pending_embeddings`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "tenant_id"}).
			AddRow(int64(1), "hello", tenantID))
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).WillReturnError(errors.New("tenant error"))
	w := NewEmbeddingWorker(mock, &failProvider{})
	w.processPendingEmbeddings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type fixedProvider struct{ vec []float32 }

func (p *fixedProvider) Generate(_ context.Context, _ string) ([]float32, error) {
	return p.vec, nil
}

func TestEmbeddingWorker_processPending_updateSuccess(t *testing.T) {
	mock := newMock(t)
	tenantID := "00000000-0000-0000-0000-000000000000"
	mock.ExpectQuery(`list_pending_embeddings`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "tenant_id"}).
			AddRow(int64(1), "hello", tenantID))
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`UPDATE memory_entries SET embedding`).WithArgs(any3()...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	prov := &fixedProvider{vec: []float32{0.1, 0.2, 0.3}}
	w := NewEmbeddingWorker(mock, prov)
	w.processPendingEmbeddings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddingWorker_processPending_emptyTenantSkipsExec(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`list_pending_embeddings`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "tenant_id"}).
			AddRow(int64(2), "world", ""))
	// No set_tenant_context expected — empty tenantID skips the exec block.
	// provider.Generate still called; use failProvider so no UPDATE follows.
	w := NewEmbeddingWorker(mock, &failProvider{})
	w.processPendingEmbeddings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── PruningWorker ────────────────────────────────────────────────────────────

func TestPruningWorker_Start_exitsOnCancel(t *testing.T) {
	mock := newMock(t)
	// runOnce is called once before the select loop.
	mock.ExpectQuery(`prune_superseded_memories`).WithArgs(any1()...).
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &PruningWorker{db: mock, retentionDays: 30, interval: 6 * time.Hour}
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	waitDone(t, done)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPruningWorker_runOnce_queryError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`prune_superseded_memories`).WithArgs(any1()...).WillReturnError(errors.New("db error"))
	w := &PruningWorker{db: mock, retentionDays: 30, interval: time.Hour}
	w.runOnce()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPruningWorker_runOnce_prunesRows(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`prune_superseded_memories`).WithArgs(any1()...).
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(3))
	w := &PruningWorker{db: mock, retentionDays: 30, interval: time.Hour}
	w.runOnce()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── ExpiryWorker ─────────────────────────────────────────────────────────────

func TestExpiryWorker_runOnce_firstExecError(t *testing.T) {
	mock := newMock(t)
	// UPDATE memory_entries has no Go args — all literals in SQL.
	mock.ExpectExec(`UPDATE memory_entries`).WillReturnError(errors.New("db error"))
	w := &ExpiryWorker{db: mock, interval: time.Hour}
	w.runOnce()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExpiryWorker_runOnce_idemExecError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE memory_entries`).
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	mock.ExpectExec(`DELETE FROM idempotency_cache`).WillReturnError(errors.New("db error"))
	w := &ExpiryWorker{db: mock, interval: time.Hour}
	w.runOnce()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExpiryWorker_runOnce_success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE memory_entries`).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectExec(`DELETE FROM idempotency_cache`).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	w := &ExpiryWorker{db: mock, interval: time.Hour}
	w.runOnce()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestExpiryWorker_runOnce_enforcesExpiresAt locks in that the expiry UPDATE
// closes rows on the first-class expires_at column (not only metadata.ttl_seconds).
// Against the pre-fix SQL this expectation does not match and the test fails.
func TestExpiryWorker_runOnce_enforcesExpiresAt(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE memory_entries[\s\S]*expires_at IS NOT NULL AND expires_at <= NOW`).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`DELETE FROM idempotency_cache`).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	w := &ExpiryWorker{db: mock, interval: time.Hour}
	w.runOnce()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExpiryWorker_Start_exitsOnCancel_mock(t *testing.T) {
	// ExpiryWorker.Start does NOT call runOnce before the loop.
	w := NewExpiryWorker(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	waitDone(t, done)
}

// ─── DistillationPolicyEngine ─────────────────────────────────────────────────

func TestDistillationPolicyEngine_Start_nilDB(t *testing.T) {
	e := NewDistillationPolicyEngine(nil, nil)
	e.Start(context.Background()) // nil db → returns immediately
}

func TestDistillationPolicyEngine_Start_exitsOnCancel(t *testing.T) {
	mock := newMock(t)
	e := NewDistillationPolicyEngine(mock, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { e.Start(ctx); close(done) }()
	waitDone(t, done)
}

func TestDistillationPolicyEngine_OnMemoryEvent_guards(t *testing.T) {
	var nilEng *DistillationPolicyEngine
	nilEng.OnMemoryEvent("t", "root.a") // nil receiver guard

	e1 := &DistillationPolicyEngine{db: nil}
	e1.OnMemoryEvent("t", "root.a") // nil db guard

	mock := newMock(t)
	e2 := NewDistillationPolicyEngine(mock, nil)
	e2.OnMemoryEvent("", "root.a") // empty tenantID guard
}

func TestDistillationPolicyEngine_evaluateAll_queryError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id, tenant_id::text, name`).WillReturnError(errors.New("db error"))
	e := NewDistillationPolicyEngine(mock, nil)
	e.evaluateAll(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationPolicyEngine_evaluateAll_emptyRows(t *testing.T) {
	mock := newMock(t)
	cols := []string{"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at"}
	mock.ExpectQuery(`SELECT id, tenant_id::text, name`).
		WillReturnRows(pgxmock.NewRows(cols))
	e := NewDistillationPolicyEngine(mock, nil)
	e.evaluateAll(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationPolicyEngine_evaluatePrefix_setTenantError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).WillReturnError(errors.New("tenant error"))
	e := NewDistillationPolicyEngine(mock, nil)
	err := e.evaluatePrefix(context.Background(), "tenant-id", "root")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDistillationPolicyEngine_evaluatePrefix_listPoliciesError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`SELECT id, tenant_id::text, name`).WithArgs(any2()...).
		WillReturnError(errors.New("db error"))
	e := NewDistillationPolicyEngine(mock, nil)
	err := e.evaluatePrefix(context.Background(), "tenant-id", "root")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDistillationPolicyEngine_evaluatePrefix_noPolicies_noDistWorker(t *testing.T) {
	mock := newMock(t)
	cols := []string{"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at"}
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`SELECT id, tenant_id::text, name`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows(cols))
	e := NewDistillationPolicyEngine(mock, nil) // dist=nil → TriggerForPrefix skipped
	if err := e.evaluatePrefix(context.Background(), "tenant-id", "root"); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationPolicyEngine_loadEvalState_success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\)::int`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"count", "oldest_secs"}).AddRow(5, 3600.0))
	e := NewDistillationPolicyEngine(mock, nil)
	st, err := e.loadEvalState(context.Background(), "tenant-id", "root")
	if err != nil {
		t.Fatal(err)
	}
	if st.MemoryCount != 5 {
		t.Fatalf("MemoryCount=%d", st.MemoryCount)
	}
	if st.OldestEntryAge != 3600*time.Second {
		t.Fatalf("OldestEntryAge=%v", st.OldestEntryAge)
	}
}

func TestDistillationPolicyEngine_loadEvalState_error(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\)::int`).WithArgs(any2()...).WillReturnError(errors.New("db error"))
	e := NewDistillationPolicyEngine(mock, nil)
	_, err := e.loadEvalState(context.Background(), "tenant-id", "root")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDistillationPolicyEngine_markPolicyTriggered(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE distillation_policies`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	e := NewDistillationPolicyEngine(mock, nil)
	if err := e.markPolicyTriggered(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationPolicyEngine_RecordRunFailure_nilDB(t *testing.T) {
	e := &DistillationPolicyEngine{db: nil}
	err := e.RecordRunFailure(context.Background(), 1, "oops")
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

// ─── DistillationPolicyEngine.maybeTrigger ────────────────────────────────────

func TestDistillationPolicyEngine_maybeTrigger_underThreshold(t *testing.T) {
	mock := newMock(t)
	e := NewDistillationPolicyEngine(mock, nil)
	p := model.DistillationPolicy{Enabled: true, CountThreshold: 100}
	st := PolicyEvalState{MemoryCount: 1}
	// ShouldTriggerPolicy returns false → no DB calls.
	e.maybeTrigger(context.Background(), p, st)
}

func TestDistillationPolicyEngine_maybeTrigger_triggers(t *testing.T) {
	mock := newMock(t)
	// beginRun → INSERT INTO distillation_runs RETURNING id
	mock.ExpectQuery(`INSERT INTO distillation_runs`).WithArgs(any4()...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(1)))
	// markPolicyTriggered → UPDATE distillation_policies
	mock.ExpectExec(`UPDATE distillation_policies`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// completeRun → UPDATE distillation_runs
	mock.ExpectExec(`UPDATE distillation_runs`).WithArgs(any3()...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	e := NewDistillationPolicyEngine(mock, nil)
	p := model.DistillationPolicy{
		ID: 1, TenantID: "00000000-0000-0000-0000-000000000000",
		PathPrefix: "root", Enabled: true, CountThreshold: 1,
	}
	e.maybeTrigger(context.Background(), p, PolicyEvalState{MemoryCount: 10})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationPolicyEngine_maybeTrigger_beginRunError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO distillation_runs`).WithArgs(any4()...).
		WillReturnError(errors.New("db error"))
	e := NewDistillationPolicyEngine(mock, nil)
	p := model.DistillationPolicy{Enabled: true, CountThreshold: 1}
	e.maybeTrigger(context.Background(), p, PolicyEvalState{MemoryCount: 10})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── DistillationWorker ───────────────────────────────────────────────────────

func TestDistillationWorker_TriggerForMemory_async(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).WillReturnError(errors.New("db error"))
	w := NewDistillationWorker(mock, nil)
	w.TriggerForMemory("00000000-0000-0000-0000-000000000000", "root.test")
	time.Sleep(100 * time.Millisecond)
}

func TestDistillationWorker_TriggerForPrefix_async(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).WillReturnError(errors.New("db error"))
	w := NewDistillationWorker(mock, nil)
	w.TriggerForPrefix("00000000-0000-0000-0000-000000000000", "root")
	time.Sleep(100 * time.Millisecond)
}

func TestDistillationWorker_runJobWithPrefix_setTenantError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).WillReturnError(errors.New("db error"))
	w := NewDistillationWorker(mock, nil)
	w.runDistillationJobWithPrefix("00000000-0000-0000-0000-000000000000", "root")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationWorker_runJobWithPrefix_queryError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`FROM memory_entries`).WithArgs(any3()...).WillReturnError(errors.New("db error"))
	w := NewDistillationWorker(mock, nil)
	w.runDistillationJobWithPrefix("00000000-0000-0000-0000-000000000000", "root")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationWorker_runJobWithPrefix_emptyEntries(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`FROM memory_entries`).WithArgs(any3()...).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "metadata"}))
	w := NewDistillationWorker(mock, nil)
	w.runDistillationJobWithPrefix("00000000-0000-0000-0000-000000000000", "root")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationWorker_runJobWithPrefix_noAPIKey(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`set_tenant_context`).WithArgs(any1()...).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`FROM memory_entries`).WithArgs(any3()...).
		WillReturnRows(pgxmock.NewRows([]string{"id", "content", "metadata"}).
			AddRow(int64(1), "hello world", []byte(`{}`)))
	// apiKey == "" → skipped after fetching entries (no LLM call).
	w := NewDistillationWorker(mock, nil)
	w.runDistillationJobWithPrefix("00000000-0000-0000-0000-000000000000", "root")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationWorker_hasDuplicateDistillation_noRows(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id FROM distilled_knowledge`).WithArgs(any3()...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}))
	w := NewDistillationWorker(mock, nil)
	dup, err := w.hasDuplicateDistillation(context.Background(), "tenant", "root.distilled", []int64{1, 2})
	if err != nil || dup {
		t.Fatalf("dup=%v err=%v", dup, err)
	}
}

func TestDistillationWorker_hasDuplicateDistillation_found(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id FROM distilled_knowledge`).WithArgs(any3()...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(99)))
	w := NewDistillationWorker(mock, nil)
	dup, err := w.hasDuplicateDistillation(context.Background(), "tenant", "root.distilled", []int64{1})
	if err != nil || !dup {
		t.Fatalf("dup=%v err=%v", dup, err)
	}
}

func TestDistillationWorker_hasDuplicateDistillation_dbError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id FROM distilled_knowledge`).WithArgs(any3()...).
		WillReturnError(errors.New("db error"))
	w := NewDistillationWorker(mock, nil)
	_, err := w.hasDuplicateDistillation(context.Background(), "tenant", "root.distilled", []int64{1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDistillationWorker_markRunCompleted_nilDB(t *testing.T) {
	w := &DistillationWorker{db: nil, sem: make(chan struct{}, 1)}
	w.markRunCompleted(context.Background(), "tenant", 42) // nil db guard → no panic
}

func TestDistillationWorker_markRunCompleted_success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE distillation_runs`).WithArgs(any3()...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	w := &DistillationWorker{db: mock, sem: make(chan struct{}, 1)}
	w.markRunCompleted(context.Background(), "00000000-0000-0000-0000-000000000000", 42)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistillationWorker_markRunCompleted_execError(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE distillation_runs`).WithArgs(any3()...).WillReturnError(errors.New("db error"))
	w := &DistillationWorker{db: mock, sem: make(chan struct{}, 1)}
	w.markRunCompleted(context.Background(), "00000000-0000-0000-0000-000000000000", 99)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── nextDistilledVersion ─────────────────────────────────────────────────────

func TestNextDistilledVersion_firstVersion(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\)`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"max_ver"}).AddRow(0))
	ver, err := nextDistilledVersion(context.Background(), mock, "tenant", "root.distilled")
	if err != nil || ver != 1 {
		t.Fatalf("ver=%d err=%v", ver, err)
	}
}

func TestNextDistilledVersion_increment(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\)`).WithArgs(any2()...).
		WillReturnRows(pgxmock.NewRows([]string{"max_ver"}).AddRow(3))
	ver, err := nextDistilledVersion(context.Background(), mock, "tenant", "root.distilled")
	if err != nil || ver != 4 {
		t.Fatalf("ver=%d err=%v", ver, err)
	}
}

func TestNextDistilledVersion_error(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\)`).WithArgs(any2()...).WillReturnError(errors.New("db error"))
	_, err := nextDistilledVersion(context.Background(), mock, "tenant", "root.distilled")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── scanDistillationPolicy ───────────────────────────────────────────────────

func TestScanDistillationPolicy_success(t *testing.T) {
	mock := newMock(t)
	now := time.Now()
	cols := []string{"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at"}
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow(int64(1), "tenant-id", "pol1", "root", true, 10, 0, nil, nil, now, now))
	rows, err := mock.Query(context.Background(), "SELECT")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected one row")
	}
	p, err := scanDistillationPolicy(rows)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 1 || p.Name != "pol1" {
		t.Fatalf("unexpected policy: %+v", p)
	}
}

// ─── config guard ─────────────────────────────────────────────────────────────

func init() { _ = config.Config{} }
