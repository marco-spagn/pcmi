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

	openAIKey := os.Getenv("OPENAI_API_KEY")
	provider := embedding.NewOpenAIProvider(openAIKey, "text-embedding-3-large")

	embeddingWorker := worker.NewEmbeddingWorker(db, provider)
	distillationWorker := worker.NewDistillationWorker(db)

	log.Println("🚀 PCMI Worker started (Embedding + Distillation)")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go embeddingWorker.Start(ctx)
	go distillationWorker.Start(ctx)

	<-ctx.Done()
	log.Println("🛑 PCMI Worker shutting down")
}
