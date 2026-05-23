// Programma pcmi-worker: job di embedding, distillation, pruning, consolidation ed expiry, più
// subscribe al canale Redis memory_events. Health e Prometheus su :8081 (`/health`, `/metrics`).
// OpenTelemetry: stesse variabili OTLP dell'API; nome servizio default `pcmi-worker` se OTEL_SERVICE_NAME è vuoto.
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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/telemetry"
	"github.com/marco-spagn/pcmi/internal/version"
	"github.com/marco-spagn/pcmi/internal/worker"
)

const workerTracerName = "github.com/marco-spagn/pcmi/cmd/worker"

func main() {
	log.Println("🚀 PCMI Worker starting...")

	// --- Fail-fast: carica e valida config prima di aprire qualsiasi connessione ---
	cfg := config.Load()
	if err := cfg.Validate(config.WorkerRequiredFields...); err != nil {
		log.Fatalf("❌ FATAL: %v", err)
	}
	log.Printf("✅ Config loaded (DB=%s, Redis=%s)", cfg.DatabaseURL[:min(len(cfg.DatabaseURL), 40)], cfg.RedisAddr)

	ctxTelemetry := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctxTelemetry, cfg, "pcmi-worker")
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

	db := database.New(cfg.DatabaseURL)
	defer db.Close()

	event.InitRedis(cfg.RedisAddr)
	event.SetEventBackend(cfg.EventBackend)

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
		mux.Handle("/metrics", promhttp.HandlerFor(
			metrics.WorkerRegistry,
			promhttp.HandlerOpts{EnableOpenMetrics: false},
		))
		log.Println("💓 Worker HTTP on :8081 (/health, /metrics)")
		if err := http.ListenAndServe(":8081", mux); err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()

	backend := event.EventBackend()
	log.Printf("✅ Redis connected, event backend=%s", backend)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prov, err := embedding.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf("❌ FATAL embedding provider: %v", err)
	}
	if prov != nil {
		ew := worker.NewEmbeddingWorker(db, prov)
		go ew.Start(ctx)
		log.Println("✅ Embedding worker started")
	} else {
		log.Println("⚠️ OPENAI_API_KEY unset — embedding worker disabled")
	}

	distWorker := worker.NewDistillationWorker(db, cfg)
	policyEngine := worker.NewDistillationPolicyEngine(db, distWorker)
	go distWorker.Start(ctx)
	go policyEngine.Start(ctx)

	pruneWorker := worker.NewPruningWorker(db, cfg)
	go pruneWorker.Start(ctx)

	consolidationWorker := worker.NewConsolidationWorker(db)
	go consolidationWorker.Start(ctx)

	expiryWorker := worker.NewExpiryWorker(db, cfg)
	go expiryWorker.Start(ctx)

	tr := otel.Tracer(workerTracerName)
	handleMemoryEvent := func(evt event.Event, streamID string) {
		metrics.IncWorkerRedisEvent(evt.Type)
		_, span := tr.Start(context.Background(), "redis.memory_event",
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("pcmi.event.type", evt.Type),
			))
		if streamID != "" {
			span.SetAttributes(attribute.String("pcmi.stream.id", streamID))
		}
		tenantID, _ := evt.Payload["tenant_id"].(string)
		if tenantID != "" {
			span.SetAttributes(attribute.String("pcmi.tenant_id", tenantID))
		}
		switch evt.Type {
		case event.EventMemoryStored, event.EventMemoryUpdated:
			path, _ := evt.Payload["path"].(string)
			log.Printf("📨 [REDIS] %s id=%v tenant=%s path=%s → distillation policy", evt.Type, evt.Payload["id"], tenantID, path)
			policyEngine.OnMemoryEvent(tenantID, path)
			consolidationWorker.TriggerForMemory(tenantID, path)
		case event.EventMemoryRefineRequested:
			prefix, _ := evt.Payload["path_prefix"].(string)
			log.Printf("📨 [REDIS] refine.requested tenant=%s prefix=%s", tenantID, prefix)
			distWorker.TriggerForPrefix(tenantID, prefix)
		}
		span.End()
	}

	if backend == event.BackendPubSub {
		redisEvents := event.SubscribeEvents()
		go func() {
			for evt := range redisEvents {
				handleMemoryEvent(evt, "")
			}
		}()
	} else {
		consumer := event.NewWorkerStreamConsumer()
		if err := consumer.EnsureGroup(ctx); err != nil {
			log.Fatalf("❌ stream consumer group: %v", err)
		}
		streamHandler := func(_ context.Context, evt event.Event, streamID string) error {
			handleMemoryEvent(evt, streamID)
			return nil
		}
		consumer.StartPendingRecovery(ctx, streamHandler)
		go func() {
			if err := consumer.Consume(ctx, streamHandler); err != nil && ctx.Err() == nil {
				log.Printf("❌ stream consumer stopped: %v", err)
			}
		}()
		log.Printf("✅ Subscribed to stream %s (group %s)", event.StreamKey, event.WorkerConsumerGroup)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 Shutting down worker...")
	cancel()
	time.Sleep(2 * time.Second)
	log.Println("👋 Worker stopped")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
