package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/log"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

const consolidationMinEntries = 3

// ConsolidationWorker merges related memories under a path prefix into a consolidated entry.
type ConsolidationWorker struct {
	db   *pgxpool.Pool
	repo *repository.MemoryRepository
	mu   sync.Mutex
}

func NewConsolidationWorker(db *pgxpool.Pool) *ConsolidationWorker {
	return &ConsolidationWorker{db: db, repo: repository.NewMemoryRepository(db, nil)}
}

func (w *ConsolidationWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processPending(ctx)
		}
	}
}

func (w *ConsolidationWorker) TriggerForMemory(tenantID, path string) {
	if tenantID == "" || path == "" {
		return
	}
	prefix := parentPrefix(path)
	if prefix == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = w.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)
		if err := w.tryConsolidate(ctx, tenantID, prefix); err != nil {
			log.Warn("consolidation trigger failed", "err", err)
		}
	}()
}

func (w *ConsolidationWorker) processPending(ctx context.Context) {
	rows, err := w.db.Query(ctx, `
		SELECT id, tenant_id::text, path_prefix
		FROM consolidation_runs
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var tenantID, prefix string
		if err := rows.Scan(&id, &tenantID, &prefix); err != nil {
			continue
		}
		_, _ = w.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)
		if err := w.runConsolidation(ctx, tenantID, prefix, id); err != nil {
			_, _ = w.db.Exec(ctx, `UPDATE consolidation_runs SET status='failed', error_message=$2, completed_at=NOW() WHERE id=$1`, id, err.Error())
		}
	}
}

func (w *ConsolidationWorker) tryConsolidate(ctx context.Context, tenantID, prefix string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	count, err := w.countActiveUnderPrefix(ctx, tenantID, prefix)
	if err != nil || count < consolidationMinEntries {
		return err
	}

	var pending int
	_ = w.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM consolidation_runs
		WHERE tenant_id = $1::uuid AND path_prefix = $2 AND status = 'pending'`,
		tenantID, prefix).Scan(&pending)
	if pending > 0 {
		return nil
	}

	_, err = w.db.Exec(ctx, `
		INSERT INTO consolidation_runs (tenant_id, path_prefix, source_entry_ids, consolidated_path, status)
		VALUES ($1::uuid, $2, ARRAY[]::bigint[], $3, 'pending')`,
		tenantID, prefix, prefix+".consolidated")
	return err
}

func (w *ConsolidationWorker) runConsolidation(ctx context.Context, tenantID, prefix string, runID int64) error {
	entries, err := w.repo.Retrieve(ctx, model.RetrieveRequest{PathPrefix: prefix, Limit: 50}, tenantID, nil)
	if err != nil {
		return err
	}
	if len(entries) < consolidationMinEntries {
		return fmt.Errorf("not enough entries")
	}

	var ids []int64
	var parts []string
	for _, e := range entries {
		if strings.HasSuffix(e.Path, ".consolidated") {
			continue
		}
		ids = append(ids, e.ID)
		parts = append(parts, e.Content)
	}
	if len(ids) < consolidationMinEntries {
		return fmt.Errorf("not enough non-consolidated entries")
	}

	consolidatedPath := prefix + ".consolidated"
	content := strings.Join(parts, "\n---\n")
	if len(content) > 16000 {
		content = content[:16000]
	}

	storeReq := model.StoreRequest{
		Path:     consolidatedPath,
		Content:  content,
		Metadata: map[string]interface{}{"consolidated": true, "source_entry_ids": ids},
		Tags:     []string{"consolidated"},
	}
	id, ver, _, err := w.repo.Store(ctx, storeReq, tenantID)
	if err != nil {
		return err
	}

	_, err = w.db.Exec(ctx, `
		UPDATE consolidation_runs
		SET status='completed', source_entry_ids=$2, consolidated_entry_id=$3, completed_at=NOW()
		WHERE id=$1`,
		runID, ids, id)
	if err != nil {
		return err
	}
	log.Info("memories consolidated", "tenant", tenantID, "prefix", prefix, "entry_id", id, "version", ver, "sources", len(ids))
	return nil
}

func (w *ConsolidationWorker) countActiveUnderPrefix(ctx context.Context, tenantID, prefix string) (int, error) {
	var n int
	err := w.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM memory_entries
		WHERE tenant_id = $1::uuid AND path <@ $2::ltree AND valid_to IS NULL
		  AND NOT path::text LIKE '%.consolidated'`,
		tenantID, prefix).Scan(&n)
	return n, err
}

func parentPrefix(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "root" {
		return "root"
	}
	if i := strings.LastIndex(path, "."); i > 0 {
		return path[:i]
	}
	return "root"
}
