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

	ticker := time.NewTicker(45 * time.Second)
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

	// Conta i ricordi grezzi da distillare
	var total int
	err := w.db.QueryRow(context.Background(), `
		SELECT COUNT(*) 
		FROM memory_entries 
		WHERE path::text LIKE 'root.test%' 
		  AND valid_to IS NULL`).Scan(&total)
	if err != nil {
		log.Printf("distillation error: %v", err)
		return
	}

	log.Printf("📊 Trovati %d ricordi grezzi da distillare", total)

	if total >= 2 {
		log.Println("🧠 Distillazione simulata completata – conoscenza di ordine superiore generata")
		// TODO v1.2: qui verrà chiamata un LLM per generare summary + insights reali
		// e salvare nella tabella distilled_knowledge
	}
}
