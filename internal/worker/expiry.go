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
	tag, err := w.db.Exec(ctx, `
		UPDATE memory_entries
		SET valid_to = NOW()
		WHERE valid_to IS NULL
		  AND (
		        (expires_at IS NOT NULL AND expires_at <= NOW())
		     OR (metadata ? 'ttl_seconds'
		         AND created_at + (metadata->>'ttl_seconds')::int * interval '1 second' < NOW())
		  )`)
	if err != nil {
		log.Printf("expiry error: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Printf("Expired %d memories", tag.RowsAffected())
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
