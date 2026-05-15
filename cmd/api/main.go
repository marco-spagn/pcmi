package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/handler"
	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/webhook"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	log.Println("🚀 PCMI API v1.14 starting...")

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
		AppName: "PCMI API v1.14",
	})

	app.Use(metrics.Middleware())
	app.Use(middleware.APIKeyMiddleware(db))
	app.Use(middleware.RateLimitMiddleware())
	app.Use(middleware.NewAuditMiddleware(db).Middleware())

	app.Get("/metrics", func(c *fiber.Ctx) error {
		fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())(c.Context())
		return nil
	})

	handler.SetupMemoryRoutes(app, db)
	handler.SetupAdminRoutes(app, db)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": "v1.14.0"})
	})

	grpcserver.Start(db, memSvc)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("✅ PCMI API v1.14 started on port %s (gRPC, batch, admin, metrics, consolidation)", port)
	log.Fatal(app.Listen(":" + port))
}
