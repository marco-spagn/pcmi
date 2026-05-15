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
	log.Println("🚀 PCMI API v1.15 starting...")

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
	webhookDispatch := webhook.NewDispatcher(db)
	event.SetWebhookNotifier(webhookDispatch.NotifyMatching)

	repo := repository.NewMemoryRepository(db)
	var embed embedding.Provider
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		embed = embedding.NewOpenAIProvider(k, os.Getenv("EMBEDDING_MODEL"))
	}
	memSvc := service.NewMemoryService(repo, embed)

	app := fiber.New(fiber.Config{
		AppName: "PCMI API v1.15",
	})

	app.Use(metrics.Middleware())
	app.Use(middleware.APIKeyMiddleware(db))
	app.Use(middleware.RateLimitMiddleware())
	app.Use(middleware.NewAuditMiddleware(db).Middleware())

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(
		metrics.Registry,
		promhttp.HandlerOpts{EnableOpenMetrics: false},
	)))

	handler.SetupMemoryRoutes(app, db)
	handler.SetupAdminRoutes(app, db)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": "v1.15.0"})
	})

	grpcserver.Start(db, memSvc)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("✅ PCMI API v1.15 started on port %s (refine, lineage, links, stats, TTL)", port)
	log.Fatal(app.Listen(":" + port))
}
