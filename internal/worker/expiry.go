package worker

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
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
	log.Printf("🕐 Expiry worker started (interval=%s)", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Expiry worker stopped")
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
		log.Printf("❌ expiry error: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Printf("🕐 Expired %d memories", tag.RowsAffected())
	}
}
