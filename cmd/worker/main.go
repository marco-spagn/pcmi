package worker

import (
	"context"
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
)

func StartDistillationWorker(cfg config.Config) {
	db := database.New(cfg.DatabaseURL) // ora usato (anche se solo per stub)
	log.Println("✅ PCMI Distillation Worker started – Temporal + Kafka ready")

	// Stub del worker (pronto per estendere con Temporal/Kafka)
	go func() {
		for {
			select {
			case <-time.After(30 * time.Second):
				log.Println("🔄 Running scheduled distillation on subtree root.*")
				// Qui andrà la logica completa di distillation (v1.1)
			case <-context.Background().Done():
				return
			}
		}
	}()
}
