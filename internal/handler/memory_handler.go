package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/service"
)

func SetupMemoryRoutes(app *fiber.App, db *pgxpool.Pool) {
	svc := service.NewMemoryService(db)

	api := app.Group("/v1")
	api.Post("/memories", svc.Store)
	api.Post("/retrieve", svc.Retrieve)

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": "1.0"})
	})
}
