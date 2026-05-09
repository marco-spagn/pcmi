package worker

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DistillationWorker struct {
	db *pgxpool.Pool
}

func NewDistillationWorker(db *pgxpool.Pool) *DistillationWorker {
	return &DistillationWorker{db: db}
}

func (w *DistillationWorker) Start(ctx context.Context) {
	log.Println("🚀 Distillation Engine started – raffinamento automatico conoscenza")

	ticker := time.NewTicker(60 * time.Second) // ogni 60 secondi
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Distillation worker stopped")
			return
		case <-ticker.C:
			w.runDistillationJob()
		}
	}
}

func (w *DistillationWorker) runDistillationJob() {
	log.Println("🔄 Avvio job di distillation su subtree root.test...")

	// TODO v1.2: qui chiameremo un LLM per generare summary + insights
	// Per ora logghiamo solo i record da distillare
	query := `
		SELECT COUNT(*) as total 
		FROM memory_entries 
		WHERE path::text LIKE 'root.test%' AND valid_to IS NULL`

	var total int
	err := w.db.QueryRow(context.Background(), query).Scan(&total)
	if err != nil {
		log.Printf("distillation error: %v", err)
		return
	}

	log.Printf("📊 Trovati %d ricordi grezzi da distillare sotto root.test", total)

	// In futuro qui salveremo nella tabella distilled_knowledge
	if total > 5 {
		log.Println("✅ Distillation simulata completata – conoscenza di ordine superiore generata")
	}
}
