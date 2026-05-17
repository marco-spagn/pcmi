// Programma pcmi-api: server HTTP (Fiber), stream SSE, eventuale gRPC MemoryService e endpoint
// Prometheus. Avvio da cmd/api; configurazione via variabili d'ambiente (vedi .env.example e docs/CODEBASE.md).
// Readiness: GET /ready e GET /v1/ready (ping Postgres + Redis, senza API key).
// Optional DATABASE_READ_URL: PostgreSQL read replica per query di lettura (retrieve, stats, ecc.).
// Optional OpenTelemetry: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT / OTEL_EXPORTER_OTLP_ENDPOINT (vedi .env.example).
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
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/handler"
	metrics "github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/middleware"
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
	log.Println("🚀 PCMI API " + version.Tag + " starting...")

	// --- Fail-fast: carica e valida config prima di aprire qualsiasi connessione ---
	cfg := config.Load()
	if err := cfg.Validate(config.APIRequiredFields...); err != nil {
		log.Fatalf("❌ FATAL: %v", err)
	}
	log.Printf("✅ Config loaded (DB=%s, Redis=%s, Port=%s)", cfg.DatabaseURL[:min(len(cfg.DatabaseURL), 40)], cfg.RedisAddr, cfg.APIPort)

	ctx := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctx, "pcmi-api")
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
	webhookDispatch := webhook.NewDispatcher(db)
	event.SetWebhookNotifier(webhookDispatch.NotifyMatching)

	repo := repository.NewMemoryRepository(db, pools.Read)
	var embed embedding.Provider
	if cfg.OpenAIAPIKey != "" {
		embed = embedding.NewOpenAIProvider(cfg.OpenAIAPIKey, cfg.EmbeddingModel)
	}
	memSvc := service.NewMemoryService(repo, embed)

	app := fiber.New(fiber.Config{
		AppName: "PCMI API " + version.Tag,
	})

	app.Use(otelfiber.Middleware(otelfiber.WithNext(skipTracePath)))
	app.Use(metrics.Middleware())
	app.Use(middleware.APIKeyMiddleware(db))
	app.Use(middleware.RateLimitMiddleware())
	app.Use(middleware.NewAuditMiddleware(db).Middleware())

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(
		metrics.Registry,
		promhttp.HandlerOpts{EnableOpenMetrics: false},
	)))

	handler.RegisterReadyRoutes(app, db)
	handler.SetupMemoryRoutes(app, db, pools.Read)
	handler.SetupAdminRoutes(app, db)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": version.Tag})
	})

	grpcserver.Start(db, pools.Read, memSvc)

	log.Printf("✅ PCMI API %s started on port %s (/v1/ready per readiness)", version.Tag, cfg.APIPort)
	if pools.Read != nil {
		log.Println("📖 DATABASE_READ_URL attivo: carico di lettura su replica")
	}
	addr := ":" + cfg.APIPort
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Printf("🔒 TLS enabled (cert=%s)", cfg.TLSCertFile)
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
