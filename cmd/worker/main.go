// Programma pcmi-worker: job di embedding, distillation, pruning, consolidation ed expiry, più
// subscribe al canale Redis memory_events. Richiede DATABASE_URL e REDIS_ADDR; health su :8081.
// OpenTelemetry: stesse variabili OTLP dell’API; nome servizio default `pcmi-worker` se OTEL_SERVICE_NAME è vuoto.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/telemetry"
	"github.com/marco-spagn/pcmi/internal/version"
	"github.com/marco-spagn/pcmi/internal/worker"
)

const workerTracerName = "github.com/marco-spagn/pcmi/cmd/worker"

func main() {
	log.Println("🚀 PCMI Worker starting...")

	ctxTelemetry := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctxTelemetry, "pcmi-worker")
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer func() {
		sdCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := shutdownTelemetry(sdCtx); e != nil {
			log.Printf("telemetry shutdown: %v", e)
		}
	}()

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

	// Health check server (port 8081) with DB pool metrics
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			stats := db.Stat()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"status":"healthy","service":"worker","version":"%s","pool":{"total_conns":%d,"idle_conns":%d,"acquired_conns":%d}}`,
				version.Tag,
				stats.TotalConns(), stats.IdleConns(), stats.AcquiredConns())
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

	pruneWorker := worker.NewPruningWorker(db)
	go pruneWorker.Start(ctx)

	consolidationWorker := worker.NewConsolidationWorker(db)
	go consolidationWorker.Start(ctx)

	expiryWorker := worker.NewExpiryWorker(db)
	go expiryWorker.Start(ctx)

	tr := otel.Tracer(workerTracerName)
	// Subscribe to Redis events (API publishes memory.stored after store)
	redisEvents := event.SubscribeEvents()
	go func() {
		for evt := range redisEvents {
			_, span := tr.Start(context.Background(), "redis.memory_event",
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("pcmi.event.type", evt.Type),
				))
			tenantID, _ := evt.Payload["tenant_id"].(string)
			if tenantID != "" {
				span.SetAttributes(attribute.String("pcmi.tenant_id", tenantID))
			}
			switch evt.Type {
			case event.EventMemoryStored, event.EventMemoryUpdated:
				path, _ := evt.Payload["path"].(string)
				log.Printf("📨 [REDIS] %s id=%v tenant=%s path=%s → distillation", evt.Type, evt.Payload["id"], tenantID, path)
				distWorker.TriggerForMemory(tenantID, path)
				consolidationWorker.TriggerForMemory(tenantID, path)
			case event.EventMemoryRefineRequested:
				prefix, _ := evt.Payload["path_prefix"].(string)
				log.Printf("📨 [REDIS] refine.requested tenant=%s prefix=%s", tenantID, prefix)
				distWorker.TriggerForPrefix(tenantID, prefix)
			}
			span.End()
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
