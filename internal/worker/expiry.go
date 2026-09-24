package worker

import (
	"context"
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

// ExpiryWorker periodically soft-closes memories whose TTL has passed.
type ExpiryWorker struct {
	db       workerDB
	interval time.Duration
}

func NewExpiryWorker(db workerDB, cfg *config.Config) *ExpiryWorker {
	interval := time.Hour
	if cfg != nil && cfg.ExpiryIntervalSecs > 0 {
		interval = time.Duration(cfg.ExpiryIntervalSecs) * time.Second
	}
	return &ExpiryWorker{db: db, interval: interval}
}

func (w *ExpiryWorker) Start(ctx context.Context) {
	log.Printf("Expiry worker started (interval=%s)", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("Expiry worker stopped")
			return
		case <-ticker.C:
			w.runOnce()
		}
	}
}

func (w *ExpiryWorker) runOnce() {
	ctx := context.Background()
	// Close rows whose TTL has passed. Two TTL mechanisms are supported:
	//   1. The first-class expires_at column (set via the API/gRPC expires_at
	//      field, indexed by migration 011). This was previously ignored, so
	//      memories stored with an explicit expiry never closed — see
	//      docs/WORKERS-AND-EVENTS.md ("Closes rows with past expires_at").
	//   2. The legacy metadata.ttl_seconds convention (relative to created_at).
	// Both are evaluated in a single UPDATE so expiry stays one round-trip.
	//
	// metadata is free-form client JSONB, so ttl_seconds may be any JSON value.
	// A naked (metadata->>'ttl_seconds')::int aborts the ENTIRE statement when any
	// current row carries a non-integer (e.g. "abc", 1.5, true) or out-of-range
	// value — one tenant could then disable expiry for every tenant. The CASE
	// guards the cast (AND evaluation order is not guaranteed in Postgres) and the
	// ^[0-9]{1,9}$ pattern admits only plain integers within int4 range (max
	// 999,999,999s ≈ 31y); malformed/oversized TTLs are ignored, not fatal.
	tag, err := w.db.Exec(ctx, `
		UPDATE memory_entries
		SET valid_to = NOW()
		WHERE valid_to IS NULL
		  AND (
		        (expires_at IS NOT NULL AND expires_at <= NOW())
		     OR (metadata ? 'ttl_seconds'
		         AND CASE
		               WHEN metadata->>'ttl_seconds' ~ '^[0-9]{1,9}$'
		               THEN created_at + (metadata->>'ttl_seconds')::int * interval '1 second' < NOW()
		               ELSE false
		             END)
		  )`)
	if err != nil {
		log.Printf("expiry error: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Printf("Expired %d memories", tag.RowsAffected())
	}

	// Namespace retention (migration 028): soft-close current memories older
	// than the max_age_days of their most specific retention policy.
	var aged int
	err = w.db.QueryRow(ctx, `SELECT expire_memories_by_retention_policy()`).Scan(&aged)
	switch {
	case isUndefinedFunction(err):
		// Pre-028 schema: no namespace retention yet.
	case err != nil:
		log.Printf("retention policy expiry: %v", err)
	case aged > 0:
		log.Printf("Retention policy closed %d memories", aged)
	}

	idemTag, err := w.db.Exec(ctx, `DELETE FROM idempotency_cache WHERE expires_at <= NOW()`)
	if err != nil {
		log.Printf("idempotency cache cleanup: %v", err)
		return
	}
	if idemTag.RowsAffected() > 0 {
		log.Printf("Purged %d expired idempotency cache rows", idemTag.RowsAffected())
	}
}
