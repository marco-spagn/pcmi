package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/worker"
)

func main() {
	log.Println("🚀 PCMI Worker starting...")

	// Database
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pcmi:pcmi@postgres:5432/pcmi?sslmode=disable"
	}

	db, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Health server on :8081
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

	// Start distillation worker
	distWorker := worker.NewDistillationWorker(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go distWorker.Start(ctx)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 Shutting down worker...")
	cancel()
	time.Sleep(2 * time.Second)
	log.Println("👋 Worker stopped")
}
