package worker

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	// TODO: sostituire con adapter reale (OpenAI, Voyage, Ollama, ecc.)
)

type EmbeddingWorker struct {
	db *pgxpool.Pool
}

func NewEmbeddingWorker(db *pgxpool.Pool) *EmbeddingWorker {
	return &EmbeddingWorker{db: db}
}

func (w *EmbeddingWorker) Start(ctx context.Context) {
	log.Println("🚀 Embedding Background Worker started")

	ticker := time.NewTicker(30 * time.Second) // ogni 30 secondi
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
	query := `
		SELECT id, content 
		FROM memory_entries 
		WHERE embedding IS NULL 
		LIMIT 10`

	rows, err := w.db.Query(context.Background(), query)
	if err != nil {
		log.Printf("embedding worker error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			continue
		}

		// TODO: chiamare modello embedding reale
		// Per ora simuliamo un vettore (da sostituire)
		fakeEmbedding := make([]float32, 1536)
		for i := range fakeEmbedding {
			fakeEmbedding[i] = 0.1 // placeholder
		}

		update := `UPDATE memory_entries SET embedding = $1 WHERE id = $2`
		_, err := w.db.Exec(context.Background(), update, pgvector.NewVector(fakeEmbedding), id)
		if err != nil {
			log.Printf("failed to update embedding for id %d: %v", id, err)
		} else {
			log.Printf("✅ Embedding generato per memory id %d", id)
		}
	}
}
