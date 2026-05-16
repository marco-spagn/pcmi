// Programma pcmi-api: server HTTP (Fiber), stream SSE, eventuale gRPC MemoryService e endpoint
// Prometheus. Avvio da cmd/api; configurazione via variabili d’ambiente (vedi .env.example e docs/CODEBASE.md).
// Readiness: GET /ready e GET /v1/ready (ping Postgres + Redis, senza API key).
// Optional DATABASE_READ_URL: PostgreSQL read replica per query di lettura (retrieve, stats, ecc.).
package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/handler"
	metrics "github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/webhook"
)

func main() {
	log.Println("🚀 PCMI API v1.18 starting...")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pcmi:pcmi@postgres:5432/pcmi?sslmode=disable"
	}

	readURL := os.Getenv("DATABASE_READ_URL")
	pools := database.NewPools(dbURL, readURL)
	defer pools.Close()
	db := pools.Primary

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	event.InitRedis(redisAddr)
	webhookDispatch := webhook.NewDispatcher(db)
	event.SetWebhookNotifier(webhookDispatch.NotifyMatching)

	repo := repository.NewMemoryRepository(db, pools.Read)
	var embed embedding.Provider
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		embed = embedding.NewOpenAIProvider(k, os.Getenv("EMBEDDING_MODEL"))
	}
	memSvc := service.NewMemoryService(repo, embed)

	app := fiber.New(fiber.Config{
		AppName: "PCMI API v1.18",
	})

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
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": "v1.18.0"})
	})

	grpcserver.Start(db, memSvc)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("✅ PCMI API v1.18 started on port %s (/v1/ready per readiness)", port)
	if pools.Read != nil {
		log.Println("📖 DATABASE_READ_URL attivo: carico di lettura su replica")
	}
	log.Fatal(app.Listen(":" + port))
}
