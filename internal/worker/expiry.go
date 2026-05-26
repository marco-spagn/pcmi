package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/log"
)

// ExpiryWorker periodically soft-closes memories whose TTL has passed.
type ExpiryWorker struct {
	db       *pgxpool.Pool
	interval time.Duration
}

func NewExpiryWorker(db *pgxpool.Pool, cfg *config.Config) *ExpiryWorker {
	interval := time.Hour
	if cfg != nil && cfg.ExpiryIntervalSecs > 0 {
		interval = time.Duration(cfg.ExpiryIntervalSecs) * time.Second
	}
	return &ExpiryWorker{db: db, interval: interval}
}

func (w *ExpiryWorker) Start(ctx context.Context) {
	log.Info("expiry worker started", "interval", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("expiry worker stopped")
			return
		case <-ticker.C:
			w.runOnce()
		}
	}
}

func (w *ExpiryWorker) runOnce() {
	ctx := context.Background()
	tag, err := w.db.Exec(ctx, `
		UPDATE memory_entries
		SET valid_to = NOW()
		WHERE valid_to IS NULL
		  AND metadata ? 'ttl_seconds'
		  AND created_at + (metadata->>'ttl_seconds')::int * interval '1 second' < NOW()`)
	if err != nil {
		log.Error("expiry worker execution failed", "err", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Info("memories expired", "count", tag.RowsAffected())
	}

	idemTag, err := w.db.Exec(ctx, `DELETE FROM idempotency_cache WHERE expires_at <= NOW()`)
	if err != nil {
		log.Error("idempotency cache cleanup failed", "err", err)
		return
	}
	if idemTag.RowsAffected() > 0 {
		log.Info("idempotency cache rows purged", "count", idemTag.RowsAffected())
	}
}
