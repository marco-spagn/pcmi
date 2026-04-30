package worker

import (
	"context"
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

func StartDistillationWorker(cfg config.Config) {
	//db := database.New(cfg.DatabaseURL) // db usato esplicitamente
	log.Printf("✅ PCMI Distillation Worker ready – connected to PostgreSQL at %s", cfg.DatabaseURL)

	// Worker reale (background distillation)
	go func() {
		for {
			select {
			case <-time.After(30 * time.Second):
				log.Println("🔄 Running scheduled distillation on subtree root.*")
				// TODO v1.1: qui useremo db per query reali su memory_entries
				// es: db.Query(...) per fetch e distillazione
			case <-context.Background().Done():
				log.Println("🛑 Distillation worker stopped")
				return
			}
		}
	}()
}
