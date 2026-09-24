package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/marco-spagn/pcmi/internal/config"
)

// pgUndefinedFunction is SQLSTATE 42883: the function does not exist (e.g. a
// worker rolled out before migration 028 was applied).
const pgUndefinedFunction = "42883"

func isUndefinedFunction(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUndefinedFunction
}

// PruningWorker periodically deletes superseded memory rows past retention.
// retentionDays is the global default; per-namespace retention_policies
// (migration 028) override it for their path prefix.
type PruningWorker struct {
	db            workerDB
	retentionDays int
	interval      time.Duration
}

func NewPruningWorker(db workerDB, cfg *config.Config) *PruningWorker {
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
	log.Printf("Pruning worker started (retention=%dd, interval=%s)", w.retentionDays, w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Println("Pruning worker stopped")
			return
		case <-ticker.C:
			w.runOnce()
		}
	}
}

func (w *PruningWorker) runOnce() {
	ctx := context.Background()
	var n int
	err := w.db.QueryRow(ctx, "SELECT prune_superseded_memories_with_policies($1)", w.retentionDays).Scan(&n)
	if isUndefinedFunction(err) {
		// Pre-028 schema: global retention only.
		err = w.db.QueryRow(ctx, "SELECT prune_superseded_memories($1)", w.retentionDays).Scan(&n)
	}
	if err != nil {
		log.Printf("pruning: %v", err)
		return
	}
	if n > 0 {
		log.Printf("Pruned %d superseded memory row(s)", n)
	}
}
