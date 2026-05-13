package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/handler"
)

func main() {
	log.Println("🚀 PCMI API starting...")

	// Database
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pcmi:pcmi@postgres:5432/pcmi?sslmode=disable"
	}

	db := database.New(dbURL)
	defer db.Close()

	// Initialize Redis
	event.InitRedis("redis:6379")

	// Setup routes
	app := fiber.New(fiber.Config{
		AppName: "PCMI API v1.3",
	})

	handler.SetupMemoryRoutes(app, db)

	// Start server
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("✅ PCMI API started on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
