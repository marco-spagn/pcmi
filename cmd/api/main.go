package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/handler"
)

func main() {
	cfg := config.Load()
	db := database.New(cfg.DatabaseURL)

	app := fiber.New(fiber.Config{
		AppName: "PCMI API",
	})

	// Memory Routes (store / retrieve)
	handler.SetupMemoryRoutes(app, db)

	// Distilled Knowledge API (nuova rotta)
	distilledHandler := handler.NewDistilledHandler(db)
	app.Get("/v1/distilled", distilledHandler.Get)

	log.Printf("🚀 PCMI API started on port %s", cfg.APIPort)
	log.Fatal(app.Listen(":" + cfg.APIPort))
}
