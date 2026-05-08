package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/worker"
)

func main() {
	cfg := config.Load()
	db := database.New(cfg.DatabaseURL)

	// Provider reale (usa OPENAI_API_KEY dall'env)
	provider := embedding.NewOpenAIProvider(
		os.Getenv("OPENAI_API_KEY"),
		"text-embedding-3-large",
	)

	embeddingWorker := worker.NewEmbeddingWorker(db, provider)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go embeddingWorker.Start(ctx)

	<-ctx.Done()
	log.Println("🛑 PCMI Worker shutting down")
}
