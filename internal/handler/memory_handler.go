package handler

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/metrics"
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
		metrics.IncStore()

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
		metrics.IncRetrieve()

		return c.JSON(result)
	})

	registerBatchRoutes(api, svc)

	dh := NewDistilledHandler(db)
	api.Get("/distilled", dh.Get)

	eh := NewEventsHandler(db)
	api.Get("/events/schemas", eh.ListSchemas)
	api.Get("/events", eh.Stream)
	api.Post("/events", middleware.RequireWriteRole, eh.Ingest)

	sh := NewSummarizeHandler(db)
	api.Post("/memories/summarize", sh.Post)

	hh := NewHistoryHandler(db)
	api.Get("/memories/history", hh.Get)

	ah := NewAuditHandler(db)
	api.Get("/audit", ah.List)

	wh := NewWebhookHandler(db)
	api.Post("/webhooks", middleware.RequireWriteRole, wh.Register)
	api.Get("/webhooks", wh.List)
	api.Get("/webhooks/dead-letter", wh.DeadLetter)

	emh := NewEmbeddingMigrateHandler(db)
	api.Post("/embeddings/migrate", middleware.RequireWriteRole, emh.Migrate)

	// Wildcard GET must be registered after all specific /memories/* routes (history, batch, etc.)
	api.Get("/memories/*", func(c *fiber.Ctx) error {
		raw := strings.TrimPrefix(c.Params("*"), "/")
		path := strings.TrimSpace(raw)
		if path == "" {
			path = c.Query("path")
		}
		if path == "" {
			return c.Status(400).JSON(fiber.Map{"error": "path is required"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		var version *int
		if v := c.Query("version"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "invalid version"})
			}
			version = &n
		}
		var asOf *time.Time
		if a := c.Query("as_of"); a != "" {
			t, err := time.Parse(time.RFC3339, a)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "invalid as_of (RFC3339)"})
			}
			asOf = &t
		}
		entry, err := svc.GetByPath(c.Context(), tenantID, path, version, asOf)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(entry)
	})

	api.Get("/health", func(c *fiber.Ctx) error {
		stats := db.Stat()
		return c.JSON(fiber.Map{
			"status":  "ok",
			"version": "v1.14.0",
			"pool": fiber.Map{
				"total_conns":    stats.TotalConns(),
				"idle_conns":     stats.IdleConns(),
				"acquired_conns": stats.AcquiredConns(),
			},
		})
	})
}
