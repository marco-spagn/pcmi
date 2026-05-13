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
	event.InitRedis("redis:6379")

	// Start distillation worker
	distWorker := worker.NewDistillationWorker(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go distWorker.Start(ctx)

	// Subscribe to Redis events
	redisEvents := event.SubscribeEvents()
	go func() {
		for evt := range redisEvents {
			if evt.Type == "memory.stored" {
				log.Printf("📨 [REDIS] Event received: %s (id=%v) → triggering distillation", evt.Type, evt.Payload["id"])
				distWorker.TriggerImmediateDistillation()
			}
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
