package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

func SetupMemoryRoutes(app *fiber.App, db *pgxpool.Pool) {
	repo := repository.NewMemoryRepository(db)
	svc := service.NewMemoryService(*repo)

	api := app.Group("/v1")

	// Solo API Key middleware (tenant viene caricato automaticamente dalla chiave)
	api.Use(middleware.APIKeyMiddleware(db))

	// Store
	api.Post("/memories", func(c *fiber.Ctx) error {
		var req model.StoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		tenantID := c.Locals(middleware.TenantContextKey).(string)

		result, err := svc.Store(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"id": result.ID, "status": "stored"})
	})

	// Retrieve
	api.Post("/retrieve", func(c *fiber.Ctx) error {
		var req model.RetrieveRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		tenantID := c.Locals(middleware.TenantContextKey).(string)

		result, err := svc.Retrieve(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(result)
	})

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": "v1.5"})
	})
}
