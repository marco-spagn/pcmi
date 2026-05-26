// Programma pcmi-api: server HTTP (Fiber), stream SSE, eventuale gRPC MemoryService e endpoint
// Prometheus. Avvio da cmd/api; configurazione via variabili d'ambiente (vedi .env.example e docs/CODEBASE.md).
// Readiness: GET /ready e GET /v1/ready (ping Postgres + Redis, senza API key).
// Optional DATABASE_READ_URL: PostgreSQL read replica per query di lettura (retrieve, stats, ecc.).
// Optional OpenTelemetry: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT / OTEL_EXPORTER_OTLP_ENDPOINT (vedi .env.example).
package main

import (
	"context"
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
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/handler"
	"github.com/marco-spagn/pcmi/internal/log"
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
	log.Info("PCMI API starting", "version", version.Tag)

	// --- Fail-fast: carica e valida config prima di aprire qualsiasi connessione ---
	cfg := config.Load()
	if err := cfg.Validate(config.APIRequiredFields...); err != nil {
		log.Fatal("config validation failed", "err", err)
	}
	if cfg.EncryptionKey != "" {
		if err := pcmicrypto.InitKey(cfg.EncryptionKey); err != nil {
			log.Fatal("encryption key initialization failed", "err", err)
		}
	}
	log.Info("config loaded", "db", log.Mask(cfg.DatabaseURL, 40), "redis", cfg.RedisAddr, "port", cfg.APIPort)
	middleware.LogMetricsScrapeAuthState(cfg.MetricsScrapeToken)

	ctx := context.Background()
	shutdownTelemetry, err := telemetry.Init(ctx, cfg, "pcmi-api")
	if err != nil {
		log.Fatal("telemetry init failed", "err", err)
	}
	defer func() {
		sdCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := shutdownTelemetry(sdCtx); e != nil {
			log.Error("telemetry shutdown failed", "err", e)
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
		log.Fatal("embedding provider init failed", "err", err)
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
		log.Fatal("memory routes setup failed", "err", err)
	}
	if err := handler.SetupSessionRoutes(app, db, pools.Read, cfg); err != nil {
		log.Fatal("session routes setup failed", "err", err)
	}
	handler.SetupAdminRoutes(app, db)
	handler.SetupDistillationPolicyRoutes(app, db)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": version.Tag})
	})

	grpcserver.Start(db, pools.Read, memSvc, cfg)

	log.Info("PCMI API started", "version", version.Tag, "port", cfg.APIPort)
	if pools.Read != nil {
		log.Info("DATABASE_READ_URL active — read traffic routed to replica")
	}
	addr := ":" + cfg.APIPort
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Info("TLS enabled", "cert", cfg.TLSCertFile)
		log.Fatal("ListenTLS failed", "err", app.ListenTLS(addr, cfg.TLSCertFile, cfg.TLSKeyFile))
	} else {
		log.Fatal("Listen failed", "err", app.Listen(addr))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
