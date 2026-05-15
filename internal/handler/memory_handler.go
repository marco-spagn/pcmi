package handler

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

func SetupMemoryRoutes(app *fiber.App, db *pgxpool.Pool) {
	repo := repository.NewMemoryRepository(db)

	var embed embedding.Provider
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		embed = embedding.NewOpenAIProvider(k, os.Getenv("EMBEDDING_MODEL"))
	}
	svc := service.NewMemoryService(repo, embed)

	api := app.Group("/v1")

	api.Post("/memories", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.StoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		tenantID := c.Locals(middleware.TenantContextKey).(string)

		result, err := svc.Store(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		resp := fiber.Map{
			"id":      result.Entry.ID,
			"status":  "stored",
			"version": result.Version,
		}
		if result.SupersededID != nil {
			resp["superseded_id"] = *result.SupersededID
		}
		return c.JSON(resp)
	})

	api.Post("/memories/rollback", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.RollbackRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Version == nil && req.AsOf == nil {
			return c.Status(400).JSON(fiber.Map{"error": "version or as_of is required"})
		}
		if req.Version != nil && req.AsOf != nil {
			return c.Status(400).JSON(fiber.Map{"error": "provide only one of version or as_of"})
		}

		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Rollback(c.Context(), &req, tenantID)
		if err != nil {
			if strings.Contains(err.Error(), "no historical version") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

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

	dh := NewDistilledHandler(db)
	api.Get("/distilled", dh.Get)

	eh := NewEventsHandler(db)
	api.Get("/events", eh.Stream)
	api.Post("/events", middleware.RequireWriteRole, eh.Ingest)

	hh := NewHistoryHandler(db)
	api.Get("/memories/history", hh.Get)

	ah := NewAuditHandler(db)
	api.Get("/audit", ah.List)

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": "v1.11.0"})
	})
}
