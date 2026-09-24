package usage

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeBatchResults struct {
	n, i    int
	failAt  int // 1-based statement index that fails; 0 = none
	closeEr error
}

func (f *fakeBatchResults) Exec() (pgconn.CommandTag, error) {
	f.i++
	if f.failAt == f.i {
		return pgconn.CommandTag{}, errors.New("boom")
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (f *fakeBatchResults) Query() (pgx.Rows, error) { return nil, errors.New("unused") }
func (f *fakeBatchResults) QueryRow() pgx.Row        { return nil }
func (f *fakeBatchResults) Close() error             { return f.closeEr }

type fakeExecer struct {
	mu      sync.Mutex
	batches []*pgx.Batch
	failAt  int
	closeEr error
	// execErr decides the row-by-row fallback result per tenant.
	execErr func(tenant string) error
	execs   []string
}

func (f *fakeExecer) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tenant, _ := args[0].(string)
	f.execs = append(f.execs, tenant)
	if f.execErr != nil {
		if err := f.execErr(tenant); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (f *fakeExecer) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, b)
	return &fakeBatchResults{n: b.Len(), failAt: f.failAt, closeEr: f.closeEr}
}

func ev(tenant, op, model string, in, out int64, at time.Time) Event {
	return Event{TenantID: tenant, Operation: op, Provider: "openai", Model: model, InputTokens: in, OutputTokens: out, At: at}
}

func TestAggregator_bucketsAndFlush(t *testing.T) {
	db := &fakeExecer{}
	a := NewAggregator(db)
	day1 := time.Date(2026, 9, 1, 23, 0, 0, 0, time.UTC)
	day2 := day1.Add(2 * time.Hour)
	a.Record(ev("t1", OpEmbedding, "m", 10, 0, day1))
	a.Record(ev("t1", OpEmbedding, "m", 5, 0, day1))
	a.Record(ev("t1", OpEmbedding, "m", 1, 0, day2)) // next UTC day
	a.Record(ev("t2", OpEmbedding, "m", 1, 0, day1))
	a.Record(ev("", OpEmbedding, "m", 99, 0, day1)) // no tenant → ignored
	a.Record(Event{TenantID: "t3", Operation: OpRerank, Provider: "p", Model: "m"})
	if a.Pending() != 4 {
		t.Fatalf("pending %d want 4", a.Pending())
	}
	a.mu.Lock()
	b := a.pending[bucketKey{"t1", "2026-09-01", OpEmbedding, "openai", "m"}]
	a.mu.Unlock()
	if b.requests != 2 || b.inputTokens != 15 {
		t.Fatalf("bucket %+v", b)
	}
	if err := a.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Pending() != 0 || len(db.batches) != 1 || db.batches[0].Len() != 4 {
		t.Fatalf("pending=%d batches=%d", a.Pending(), len(db.batches))
	}
	if err := a.Flush(context.Background()); err != nil || len(db.batches) != 1 {
		t.Fatal("empty flush must not send a batch")
	}
}

func TestAggregator_flushFailureRequeuesTransient(t *testing.T) {
	transient := func(string) error { return errors.New("connection reset") }
	for name, db := range map[string]*fakeExecer{
		"exec error":  {failAt: 1, execErr: transient},
		"close error": {closeEr: errors.New("conn"), execErr: transient},
	} {
		t.Run(name, func(t *testing.T) {
			a := NewAggregator(db)
			now := time.Now()
			a.Record(ev("t1", OpSummarize, "m", 7, 3, now))
			if err := a.Flush(context.Background()); err == nil {
				t.Fatal("expected error")
			}
			a.Record(ev("t1", OpSummarize, "m", 1, 1, now))
			a.mu.Lock()
			defer a.mu.Unlock()
			b := a.pending[bucketKey{"t1", now.UTC().Format(time.DateOnly), OpSummarize, "openai", "m"}]
			if b == nil || b.requests != 2 || b.inputTokens != 8 || b.outputTokens != 4 {
				t.Fatalf("requeued bucket %+v", b)
			}
		})
	}
}

func TestAggregator_batchFailureFallsBackRowByRow(t *testing.T) {
	db := &fakeExecer{failAt: 1, execErr: func(tenant string) error {
		if tenant == "gone" {
			return &pgconn.PgError{Code: "23503"} // FK violation: tenant deleted
		}
		return nil
	}}
	a := NewAggregator(db)
	now := time.Now()
	a.Record(ev("gone", OpEmbedding, "m", 1, 0, now))
	a.Record(ev("ok", OpEmbedding, "m", 1, 0, now))
	if err := a.Flush(context.Background()); err != nil {
		t.Fatalf("permanent row errors must not fail the flush: %v", err)
	}
	if a.Pending() != 0 || len(db.execs) != 2 {
		t.Fatalf("pending=%d execs=%v", a.Pending(), db.execs)
	}
}

func TestIsPermanent(t *testing.T) {
	for code, want := range map[string]bool{"23503": true, "22P02": true, "40001": false, "08006": false, "": false} {
		if got := isPermanent(&pgconn.PgError{Code: code}); got != want {
			t.Errorf("%q: got %v", code, got)
		}
	}
	if isPermanent(errors.New("plain")) {
		t.Error("non-pg error must be transient")
	}
}

func TestAggregator_capDropsNewKeys(t *testing.T) {
	a := NewAggregator(&fakeExecer{})
	now := time.Now()
	for i := 0; i < maxPendingBuckets; i++ {
		a.pending[bucketKey{tenantID: "t", model: strconv.Itoa(i)}] = &bucket{}
	}
	a.Record(ev("t-new", OpEmbedding, "m", 1, 0, now))
	if a.Pending() != maxPendingBuckets || a.dropped != 1 {
		t.Fatalf("pending=%d dropped=%d", a.Pending(), a.dropped)
	}
	if err := a.Flush(context.Background()); err != nil || a.dropped != 0 {
		t.Fatalf("flush err=%v dropped=%d", err, a.dropped)
	}
}

func TestAggregator_runFlushesOnCancel(t *testing.T) {
	db := &fakeExecer{}
	a := NewAggregator(db)
	a.Record(ev("t1", OpEmbedding, "m", 1, 0, time.Now()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx, 0); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
	if len(db.batches) != 1 || a.Pending() != 0 {
		t.Fatalf("final flush missing: batches=%d pending=%d", len(db.batches), a.Pending())
	}
}

func TestAggregator_runTicks(t *testing.T) {
	db := &fakeExecer{failAt: 1, execErr: func(string) error { return errors.New("down") }}
	a := NewAggregator(db)
	a.Record(ev("t1", OpEmbedding, "m", 1, 0, time.Now()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx, 10*time.Millisecond); close(done) }()
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
	if len(db.batches) < 2 {
		t.Fatalf("expected repeated flush attempts, got %d", len(db.batches))
	}
}
