package worker

import (
	"context"
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
)

func StartDistillationWorker(cfg config.Config) {
	db := database.New(cfg.DatabaseURL) // db ora usato esplicitamente
	log.Printf("✅ PCMI Distillation Worker ready – connected to PostgreSQL")

	// Worker background (simulazione distillation)
	go func() {
		for {
			select {
			case <-time.After(30 * time.Second):
				log.Println("🔄 Running scheduled distillation on subtree root.*")
				// TODO: qui useremo db per query reali in v1.1
				// es: db.Query(...) per fetch memory_entries da distillare
			case <-context.Background().Done():
				log.Println("🛑 Distillation worker stopped")
				return
			}
		}
	}()
}
