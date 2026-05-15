package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/handler"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

func main() {
	log.Println("🚀 PCMI API v1.7 starting...")

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

	app := fiber.New(fiber.Config{
		AppName: "PCMI API v1.7",
	})

	// Middlewares
	app.Use(middleware.APIKeyMiddleware(db))
	app.Use(middleware.NewAuditMiddleware(db).Middleware())

	// Routes
	handler.SetupMemoryRoutes(app, db)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "pcmi-api", "version": "v1.7"})
	})

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("✅ PCMI API v1.7 started on port %s (Audit + RBAC + hybrid retrieval)", port)
	log.Fatal(app.Listen(":" + port))
}
