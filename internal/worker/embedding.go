package worker

import (
	"context"
	"log"
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
	log.Println("🚀 Real OpenAI Embedding Background Worker started")

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Embedding worker stopped")
			return
		case <-ticker.C:
			w.processPendingEmbeddings()
		}
	}
}

func (w *EmbeddingWorker) processPendingEmbeddings() {
	ctx := context.Background()
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
		log.Printf("embedding worker query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var content, tenantID string
		if err := rows.Scan(&id, &content, &tenantID); err != nil {
			continue
		}

		ctx := context.Background()
		if tenantID != "" {
			if _, err := w.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
				log.Printf("embedding worker set tenant for id %d: %v", id, err)
				continue
			}
		}

		emb, err := w.provider.Generate(ctx, content)
		if err != nil {
			log.Printf("failed to generate embedding for id %d: %v", id, err)
			continue
		}

		_, err = w.db.Exec(ctx,
			`UPDATE memory_entries SET embedding = $1 WHERE id = $2 AND tenant_id = $3::uuid`,
			pgvector.NewVector(emb), id, tenantID)

		if err == nil {
			log.Printf("✅ Embedding generato e salvato per memory id %d", id)
		} else {
			log.Printf("failed to update embedding for id %d: %v", id, err)
		}
	}
}
