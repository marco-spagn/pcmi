package worker

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
)

// PruningWorker periodically deletes superseded memory rows past retention.
type PruningWorker struct {
	db            *pgxpool.Pool
	retentionDays int
	interval      time.Duration
}

func NewPruningWorker(db *pgxpool.Pool, cfg *config.Config) *PruningWorker {
	days := 30
	interval := 6 * time.Hour
	if cfg != nil {
		if cfg.PruneRetentionDays > 0 {
			days = cfg.PruneRetentionDays
		}
		if cfg.PruneIntervalSecs > 0 {
			interval = time.Duration(cfg.PruneIntervalSecs) * time.Second
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
