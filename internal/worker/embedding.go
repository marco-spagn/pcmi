package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/pgvector/pgvector-go"
)

type EmbeddingWorker struct {
	db       *pgxpool.Pool
	provider embedding.Provider
}

func NewEmbeddingWorker(db *pgxpool.Pool, provider embedding.Provider) *EmbeddingWorker {
	return &EmbeddingWorker{db: db, provider: provider}
}

func (w *EmbeddingWorker) Start(ctx context.Context) {
	slog.InfoContext(ctx, "embedding worker started")

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "embedding worker stopped")
			return
		case <-ticker.C:
			w.processPendingEmbeddings(ctx)
		}
	}
}

func (w *EmbeddingWorker) processPendingEmbeddings(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	query := `SELECT id, content, tenant_id::text FROM list_pending_embeddings(5)`
	rows, err := w.db.Query(ctx, query)
	if err != nil {
		// Fallback when migration 005 not applied (e.g. older volumes)
		query = `
			SELECT id, content, tenant_id::text
			FROM memory_entries
			WHERE embedding IS NULL AND valid_to IS NULL
			ORDER BY created_at ASC
			LIMIT 5`
		rows, err = w.db.Query(ctx, query)
	}
	if err != nil {
		slog.ErrorContext(ctx, "embedding worker query error", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var content, tenantID string
		if err := rows.Scan(&id, &content, &tenantID); err != nil {
			continue
		}

		rowCtx := ctx
		if tenantID != "" {
			if _, err := w.db.Exec(rowCtx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
				slog.WarnContext(rowCtx, "embedding worker set tenant failed", "id", id, "err", err)
				continue
			}
		}

		emb, err := w.provider.Generate(rowCtx, content)
		if err != nil {
			if errors.Is(err, embedding.ErrCircuitOpen) {
				slog.WarnContext(rowCtx, "embedding circuit open, skipping entry", "id", id)
			} else {
				slog.WarnContext(rowCtx, "embedding generation failed", "id", id, "err", err)
			}
			continue
		}

		_, err = w.db.Exec(rowCtx,
			`UPDATE memory_entries SET embedding = $1 WHERE id = $2 AND tenant_id = $3::uuid`,
			pgvector.NewVector(emb), id, tenantID)

		if err == nil {
			slog.InfoContext(rowCtx, "embedding saved", "id", id)
		} else {
			slog.WarnContext(rowCtx, "embedding update failed", "id", id, "err", err)
		}
	}
}
