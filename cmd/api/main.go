// Program pcmi-api: HTTP server (Fiber), SSE streams, optional gRPC MemoryService and
// Prometheus endpoint. Started from cmd/api; configured via environment variables (see .env.example and docs/CODEBASE.md).
// Readiness: GET /ready and GET /v1/ready (ping Postgres + Redis, no API key required).
// Optional DATABASE_READ_URL: PostgreSQL read replica for read queries (retrieve, stats, etc.).
// Optional OpenTelemetry: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT / OTEL_EXPORTER_OTLP_ENDPOINT (see .env.example).
package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/marco-spagn/pcmi/internal/config"
	pcmicrypto "github.com/marco-spagn/pcmi/internal/crypto"
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/graph"
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/handler"
	metrics "github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/telemetry"
	"github.com/marco-spagn/pcmi/internal/version"
	"github.com/marco-spagn/pcmi/internal/webhook"
)

func skipTracePath(c *fiber.Ctx) bool {
	p := c.Path()
	if p == "/metrics" || p == "/health" || p == "/v1/health" {
		return true
	}
	if strings.HasPrefix(p, "/ready") || strings.HasPrefix(p, "/v1/ready") {
		return true
	}
	return false
}

func main() {
	log.Println("PCMI API " + version.Tag + " starting...")

	// --- Fail-fast: load and validate config before opening any connection ---
	cfg := config.Load()
	if err := cfg.Validate(config.APIRequiredFields...); err != nil {
		log.Fatalf(" FATAL: %v", err)
	}
	if cfg.EncryptionKey != "" {
		if err := pcmicrypto.InitKey(cfg.EncryptionKey); err != nil {
			log.Fatalf(" FATAL encryption key: %v", err)
		}
	}
	log.Printf("Config loaded (DB=%s, Redis=%s, Port=%s)", cfg.DatabaseURL[:min(len(cfg.DatabaseURL), 40)], cfg.RedisAddr, cfg.APIPort)
	middleware.LogMetricsScrapeAuthState(cfg.MetricsScrapeToken)

	ctx := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctx, cfg, "pcmi-api")
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

	pools := database.NewPools(cfg.DatabaseURL, cfg.DatabaseReadURL)
	defer pools.Close()
	db := pools.Primary

	event.InitRedis(cfg.RedisAddr)
	event.SetEventBackend(cfg.EventBackend)
	webhookDispatch := webhook.NewDispatcher(db, cfg.WebhookMaxAttempts)
	event.SetWebhookNotifier(webhookDispatch.NotifyMatching)

	repo := repository.NewMemoryRepository(db, pools.Read)
	embed, err := embedding.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf(" FATAL embedding provider: %v", err)
	}
	dedupMode, _ := model.ParseDedupMode(cfg.DedupMode)
	memSvc := service.NewMemoryService(repo, embed, dedupMode)

	app := fiber.New(fiber.Config{
		AppName: "PCMI API " + version.Tag,
	})

	app.Use(otelfiber.Middleware(otelfiber.WithNext(skipTracePath)))
	app.Use(metrics.Middleware())
	app.Use(middleware.APIKeyMiddleware(db))
	app.Use(middleware.RateLimitMiddleware(cfg))
	app.Use(middleware.NewAuditMiddleware(db).Middleware())

	app.Get("/metrics", middleware.MetricsScrapeAuth(cfg.MetricsScrapeToken), adaptor.HTTPHandler(promhttp.HandlerFor(
		metrics.Registry,
		promhttp.HandlerOpts{EnableOpenMetrics: false},
	)))

	handler.RegisterReadyRoutes(app, db)
	if err := handler.SetupMemoryRoutes(app, db, pools.Read, cfg); err != nil {
		log.Fatalf(" FATAL memory routes: %v", err)
	}
	if err := handler.SetupSessionRoutes(app, db, pools.Read, cfg); err != nil {
		log.Fatalf(" FATAL session routes: %v", err)
	}
	handler.SetupAdminRoutes(app, db)
	handler.SetupDistillationPolicyRoutes(app, db)

	graphClient := graph.NewGraphClient(db)
	if graphClient.IsAvailable(ctx) {
		handler.RegisterGraphRoutes(app, graphClient)
		log.Println("🧠 Cognitive Graph (AGE) available — /v1/graph endpoints enabled")
	} else {
		log.Println("ℹ️  Cognitive Graph (AGE) not available — /v1/graph endpoints disabled")
	}

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": version.Tag})
	})

	grpcserver.Start(db, pools.Read, memSvc, cfg)

	log.Printf("PCMI API %s started on port %s (/v1/ready per readiness)", version.Tag, cfg.APIPort)
	if pools.Read != nil {
		log.Println("DATABASE_READ_URL active: read load routed to replica")
	}
	addr := ":" + cfg.APIPort
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Printf("TLS enabled (cert=%s)", cfg.TLSCertFile)
		log.Fatal(app.ListenTLS(addr, cfg.TLSCertFile, cfg.TLSKeyFile))
	} else {
		log.Fatal(app.Listen(addr))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
