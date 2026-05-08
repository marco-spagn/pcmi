package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/worker"
)

func main() {
	cfg := config.Load()
	db := database.New(cfg.DatabaseURL)

	log.Println("🚀 PCMI Worker started")

	embeddingWorker := worker.NewEmbeddingWorker(db)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go embeddingWorker.Start(ctx)

	<-ctx.Done()
	log.Println("🛑 PCMI Worker shutting down")
}
