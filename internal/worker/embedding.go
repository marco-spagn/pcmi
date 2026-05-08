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
	query := `SELECT id, content FROM memory_entries WHERE embedding IS NULL LIMIT 5`

	rows, err := w.db.Query(context.Background(), query)
	if err != nil {
		log.Printf("embedding worker query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			continue
		}

		emb, err := w.provider.Generate(context.Background(), content)
		if err != nil {
			log.Printf("failed to generate embedding for id %d: %v", id, err)
			continue
		}

		_, err = w.db.Exec(context.Background(),
			`UPDATE memory_entries SET embedding = $1 WHERE id = $2`,
			pgvector.NewVector(emb), id)

		if err == nil {
			log.Printf("✅ Embedding generato e salvato per memory id %d", id)
		} else {
			log.Printf("failed to update embedding for id %d: %v", id, err)
		}
	}
}
