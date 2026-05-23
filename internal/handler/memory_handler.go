package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/version"
)

func SetupMemoryRoutes(app *fiber.App, dbWrite, readReplica *pgxpool.Pool, cfg *config.Config) error {
	repo := repository.NewMemoryRepository(dbWrite, readReplica)

	embed, err := embedding.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("embedding provider: %w", err)
	}
	svc := service.NewMemoryService(repo, embed)

	api := app.Group("/v1")

	api.Post("/memories", middleware.RequireWriteRole, middleware.Idempotency(dbWrite), func(c *fiber.Ctx) error {
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

	api.Patch("/memories/*/importance", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		raw := strings.TrimSuffix(strings.TrimPrefix(c.Params("*"), "/"), "/importance")
		path := strings.TrimSpace(raw)
		if path == "" {
			return c.Status(400).JSON(fiber.Map{"error": "path is required"})
		}
		var req model.UpdateImportanceRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := model.ValidateImportance(req.Importance); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		if err := svc.UpdateImportance(c.Context(), tenantID, path, req.Importance); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"path":       path,
			"importance": req.Importance,
			"status":     "updated",
		})
	})

	api.Post("/memories/compact", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.CompactMemoryRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if strings.TrimSpace(req.Path) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "path is required"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		out, err := svc.Compact(c.Context(), tenantID, &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(out)
	})

	api.Post("/retrieve", func(c *fiber.Ctx) error {
		var req model.RetrieveRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		tenantID := c.Locals(middleware.TenantContextKey).(string)

		result, err := svc.Retrieve(c.Context(), &req, tenantID)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "invalid cursor") || strings.Contains(msg, "cursor pagination") || strings.Contains(msg, "cursor sort") {
				return c.Status(400).JSON(fiber.Map{"error": msg})
			}
			return c.Status(500).JSON(fiber.Map{"error": msg})
		}
		metrics.IncRetrieve()

		return c.JSON(result)
	})

	registerBatchRoutes(api, svc)

	lh := NewLineageHandler(dbWrite, readReplica)
	// Use /lineage/* — /memories/lineage is captured by the /memories/* wildcard.
	api.Get("/lineage/memory", lh.MemoryLineage)

	rfh := NewRefineHandler()
	api.Post("/memories/refine", middleware.RequireWriteRole, rfh.Post)

	lnh := NewLinksHandler(dbWrite, readReplica)
	api.Post("/memories/links", middleware.RequireWriteRole, lnh.Post)
	api.Get("/memories/links", lnh.List)

	RegisterStatsRoute(api, dbWrite, readReplica)

	dh := NewDistilledHandler(dbWrite)
	api.Get("/distilled", dh.Get)
	api.Get("/lineage/distilled/:id", lh.DistilledLineage)

	eh := NewEventsHandler(dbWrite)
	api.Get("/events/schemas", eh.ListSchemas)
	api.Get("/events", eh.Stream)
	api.Post("/events", middleware.RequireWriteRole, eh.Ingest)

	sh := NewSummarizeHandler(dbWrite, readReplica, cfg)
	api.Post("/memories/summarize", sh.Post)

	hh := NewHistoryHandler(dbWrite, readReplica)
	api.Get("/memories/history", hh.Get)

	ah := NewAuditHandler(dbWrite)
	api.Get("/audit", ah.List)

	wh := NewWebhookHandler(dbWrite)
	api.Post("/webhooks", middleware.RequireWriteRole, wh.Register)
	api.Get("/webhooks", wh.List)
	api.Get("/webhooks/dead-letter", wh.DeadLetter)

	emh := NewEmbeddingMigrateHandler(dbWrite)
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
		stats := dbWrite.Stat()
		return c.JSON(fiber.Map{
			"status":  "ok",
			"version": version.Tag,
			"pool": fiber.Map{
				"total_conns":    stats.TotalConns(),
				"idle_conns":     stats.IdleConns(),
				"acquired_conns": stats.AcquiredConns(),
			},
		})
	})
	return nil
}
