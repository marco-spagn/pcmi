package worker

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PruningWorker periodically deletes superseded memory rows past retention.
type PruningWorker struct {
	db            *pgxpool.Pool
	retentionDays int
	interval      time.Duration
}

func NewPruningWorker(db *pgxpool.Pool) *PruningWorker {
	days := 30
	if v := os.Getenv("PRUNE_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	interval := 6 * time.Hour
	if v := os.Getenv("PRUNE_INTERVAL_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = time.Duration(n) * time.Second
		}
	}
	return &PruningWorker{db: db, retentionDays: days, interval: interval}
}

func (w *PruningWorker) Start(ctx context.Context) {
	log.Printf("🧹 Pruning worker started (retention=%dd, interval=%s)", w.retentionDays, w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Pruning worker stopped")
			return
		case <-ticker.C:
			w.runOnce()
		}
	}
}

func (w *PruningWorker) runOnce() {
	ctx := context.Background()
	var n int
	err := w.db.QueryRow(ctx, "SELECT prune_superseded_memories($1)", w.retentionDays).Scan(&n)
	if err != nil {
		log.Printf("❌ pruning: %v", err)
		return
	}
	if n > 0 {
		log.Printf("🧹 Pruned %d superseded memory row(s)", n)
	}
}
