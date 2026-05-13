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
	log.Println("🚀 PCMI API v1.5 starting...")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pcmi:pcmi@postgres:5432/pcmi?sslmode=disable"
	}

	db := database.New(dbURL)
	defer db.Close()

	// Initialize Redis
	event.InitRedis("redis:6379")

	app := fiber.New(fiber.Config{
		AppName: "PCMI API v1.5",
	})

	// API Key Authentication (obbligatoria)
	app.Use(middleware.APIKeyMiddleware(db))

	// Routes
	handler.SetupMemoryRoutes(app, db)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("✅ PCMI API v1.5 started on port %s (API Key + RBAC enabled)", port)
	log.Fatal(app.Listen(":" + port))
}
