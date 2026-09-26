package usage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the subset of pgxpool.Pool the aggregator needs.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type bucketKey struct {
	tenantID  string
	day       string // YYYY-MM-DD (UTC)
	operation string
	provider  string
	model     string
}

type bucket struct {
	requests     int64
	inputTokens  int64
	outputTokens int64
}

// maxPendingBuckets bounds memory if the database is unreachable for a long
// time; further events for new keys are dropped (and counted) until a flush.
const maxPendingBuckets = 100000

// Aggregator batches Events in memory per (tenant, UTC day, operation,
// provider, model) and periodically upserts them into llm_usage_daily with
// additive counters. Events without a tenant are only counted in Prometheus.
type Aggregator struct {
	db Execer

	mu      sync.Mutex
	pending map[bucketKey]*bucket
	dropped int64
}

// NewAggregator returns an Aggregator writing to db.
func NewAggregator(db Execer) *Aggregator {
	return &Aggregator{db: db, pending: map[bucketKey]*bucket{}}
}

// Record implements Recorder.
func (a *Aggregator) Record(ev Event) {
	if ev.TenantID == "" {
		return
	}
	at := ev.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	k := bucketKey{
		tenantID:  ev.TenantID,
		day:       at.UTC().Format(time.DateOnly),
		operation: ev.Operation,
		provider:  ev.Provider,
		model:     ev.Model,
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.pending[k]
	if !ok {
		if len(a.pending) >= maxPendingBuckets {
			a.dropped++
			return
		}
		b = &bucket{}
		a.pending[k] = b
	}
	b.requests++
	b.inputTokens += ev.InputTokens
	b.outputTokens += ev.OutputTokens
}

// Pending reports the number of buffered buckets (for tests and logs).
func (a *Aggregator) Pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

const upsertSQL = `
	INSERT INTO llm_usage_daily (tenant_id, day, operation, provider, model, requests, input_tokens, output_tokens)
	VALUES ($1::uuid, $2::date, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (tenant_id, day, operation, provider, model) DO UPDATE SET
		requests      = llm_usage_daily.requests + EXCLUDED.requests,
		input_tokens  = llm_usage_daily.input_tokens + EXCLUDED.input_tokens,
		output_tokens = llm_usage_daily.output_tokens + EXCLUDED.output_tokens,
		updated_at    = NOW()`

// Flush writes buffered buckets in one batch. If the batch fails it retries
// row by row: rows rejected permanently (constraint or data errors, e.g. a
// tenant deleted since the event was recorded) are dropped and logged so they
// can never block other tenants' usage; rows that fail transiently (connection
// loss, timeout) are merged back and retried on the next flush.
func (a *Aggregator) Flush(ctx context.Context) error {
	a.mu.Lock()
	pending := a.pending
	dropped := a.dropped
	a.pending = map[bucketKey]*bucket{}
	a.dropped = 0
	a.mu.Unlock()

	if dropped > 0 {
		log.Printf("usage: dropped %d events while the buffer was full", dropped)
	}
	if len(pending) == 0 {
		return nil
	}

	b := &pgx.Batch{}
	for k, v := range pending {
		b.Queue(upsertSQL, upsertArgs(k, v)...)
	}
	br := a.db.SendBatch(ctx, b)
	var batchErr error
	for range pending {
		if _, err := br.Exec(); err != nil && batchErr == nil {
			batchErr = err
		}
	}
	if err := br.Close(); err != nil && batchErr == nil {
		batchErr = err
	}
	if batchErr == nil {
		return nil
	}

	// The batch ran as one implicit transaction, so nothing was applied.
	var requeue map[bucketKey]*bucket
	var firstErr error
	for k, v := range pending {
		_, err := a.db.Exec(ctx, upsertSQL, upsertArgs(k, v)...)
		switch {
		case err == nil:
		case isPermanent(err):
			log.Printf("usage: dropping bucket %s/%s/%s/%s: %v", k.tenantID, k.day, k.operation, k.model, err)
		default:
			if requeue == nil {
				requeue = map[bucketKey]*bucket{}
			}
			requeue[k] = v
			if firstErr == nil {
				firstErr = fmt.Errorf("usage flush (%s/%s): %w", k.tenantID, k.day, err)
			}
		}
	}
	if len(requeue) > 0 {
		a.mu.Lock()
		for k, v := range requeue {
			cur, ok := a.pending[k]
			if !ok {
				a.pending[k] = v
				continue
			}
			cur.requests += v.requests
			cur.inputTokens += v.inputTokens
			cur.outputTokens += v.outputTokens
		}
		a.mu.Unlock()
	}
	return firstErr
}

func upsertArgs(k bucketKey, v *bucket) []any {
	return []any{k.tenantID, k.day, k.operation, k.provider, k.model, v.requests, v.inputTokens, v.outputTokens}
}

// isPermanent reports whether retrying the same row can never succeed:
// SQLSTATE class 22 (data exception) or 23 (integrity constraint violation).
func isPermanent(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || len(pgErr.Code) < 2 {
		return false
	}
	return pgErr.Code[:2] == "22" || pgErr.Code[:2] == "23"
}

// Run flushes every interval until ctx is cancelled, then flushes once more
// with a short timeout so shutdown does not lose buffered usage.
func (a *Aggregator) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := a.Flush(fctx); err != nil {
				log.Printf("usage: final flush: %v", err)
			}
			cancel()
			return
		case <-t.C:
			if err := a.Flush(ctx); err != nil {
				log.Printf("usage: %v", err)
			}
		}
	}
}
