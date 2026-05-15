package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/worker"
)

func main() {
	log.Println("🚀 PCMI Worker starting...")

	// Database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pcmi:pcmi@postgres:5432/pcmi?sslmode=disable"
	}

	db := database.New(dbURL)
	defer db.Close()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	event.InitRedis(redisAddr)

	// Health check server (port 8081)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy","service":"worker"}`))
		})
		log.Println("💓 Worker health endpoint started on :8081")
		if err := http.ListenAndServe(":8081", mux); err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()

	// Initialize Redis
	log.Println("✅ Redis connected, subscribing to memory_events…")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		prov := embedding.NewOpenAIProvider(key, os.Getenv("EMBEDDING_MODEL"))
		ew := worker.NewEmbeddingWorker(db, prov)
		go ew.Start(ctx)
		log.Println("✅ Embedding worker started")
	} else {
		log.Println("⚠️ OPENAI_API_KEY unset — embedding worker disabled")
	}

	// Start distillation worker
	distWorker := worker.NewDistillationWorker(db)
	go distWorker.Start(ctx)

	// Subscribe to Redis events (API publishes memory.stored after store)
	redisEvents := event.SubscribeEvents()
	go func() {
		for evt := range redisEvents {
			if evt.Type != event.EventMemoryStored && evt.Type != event.EventMemoryUpdated {
				continue
			}
			tenantID, _ := evt.Payload["tenant_id"].(string)
			path, _ := evt.Payload["path"].(string)
			log.Printf("📨 [REDIS] memory.stored id=%v tenant=%s path=%s → distillation", evt.Payload["id"], tenantID, path)
			distWorker.TriggerForMemory(tenantID, path)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 Shutting down worker...")
	cancel()
	time.Sleep(2 * time.Second)
	log.Println("👋 Worker stopped")
}
