package worker

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpiryWorker periodically soft-closes memories whose TTL has passed.
type ExpiryWorker struct {
	db       *pgxpool.Pool
	interval time.Duration
}

func NewExpiryWorker(db *pgxpool.Pool) *ExpiryWorker {
	secs := 300
	if v := os.Getenv("EXPIRY_INTERVAL_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			secs = n
		}
	}
	return &ExpiryWorker{db: db, interval: time.Duration(secs) * time.Second}
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
			w.runOnce(ctx)
		}
	}
}

func (w *ExpiryWorker) runOnce(ctx context.Context) {
	var n int
	err := w.db.QueryRow(ctx, `SELECT expire_memory_entries()`).Scan(&n)
	if err != nil {
		log.Printf("❌ expiry job: %v", err)
		return
	}
	if n > 0 {
		log.Printf("⏰ Expired %d memories (TTL)", n)
	}
}
