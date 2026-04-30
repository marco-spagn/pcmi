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

	app := fiber.New(fiber.Config{AppName: "PCMI API"})

	handler.SetupMemoryRoutes(app, db)

	log.Fatal(app.Listen(":" + cfg.APIPort))
}
