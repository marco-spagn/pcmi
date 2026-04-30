package main

import (
	"log"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/worker"
)

func main() {
	cfg := config.Load()
	log.Printf("🚀 PCMI Worker started (Temporal + Kafka)")

	worker.StartDistillationWorker(cfg)
}
