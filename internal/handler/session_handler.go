package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

// SetupSessionRoutes registers /v1/sessions working-memory endpoints.
func SetupSessionRoutes(app *fiber.App, dbWrite, readReplica *pgxpool.Pool, cfg *config.Config) error {
	memRepo := repository.NewMemoryRepository(dbWrite, readReplica)
	sessRepo := repository.NewSessionRepository(dbWrite, readReplica)
	embed, err := embedding.NewFromConfig(cfg)
	if err != nil {
		return err
	}
	memSvc := service.NewMemoryService(memRepo, embed)
	svc := service.NewSessionService(sessRepo, memSvc)

	api := app.Group("/v1")

	api.Post("/sessions", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.CreateSessionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		sess, err := svc.Create(c.Context(), tenantID, &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(sess)
	})

	api.Post("/sessions/:id/memories", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		sessionID := strings.TrimSpace(c.Params("id"))
		if sessionID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "session id is required"})
		}
		var req model.SessionStoreMemoryRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Content) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "path and content are required"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.StoreMemory(c.Context(), tenantID, sessionID, &req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			if strings.Contains(err.Error(), "session ended") {
				return c.Status(409).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"id":         result.Entry.ID,
			"session_id": sessionID,
			"path":       result.Entry.Path,
			"status":     "stored",
			"version":    result.Version,
		})
	})

	api.Get("/sessions/:id/memories", func(c *fiber.Ctx) error {
		sessionID := strings.TrimSpace(c.Params("id"))
		if sessionID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "session id is required"})
		}
		limit := 50
		if v := c.Query("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return c.Status(400).JSON(fiber.Map{"error": "invalid limit"})
			}
			limit = n
		}
		includeLongTerm := c.Query("include_long_term") == "true" || c.Query("include_long_term") == "1"
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		out, err := svc.ListMemories(c.Context(), tenantID, sessionID, c.Query("path_prefix"), limit, includeLongTerm)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(out)
	})

	api.Post("/sessions/:id/promote", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		sessionID := strings.TrimSpace(c.Params("id"))
		if sessionID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "session id is required"})
		}
		var req model.PromoteSessionRequest
		_ = c.BodyParser(&req)
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		out, err := svc.Promote(c.Context(), tenantID, sessionID, &req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			if strings.Contains(err.Error(), "session ended") {
				return c.Status(409).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(out)
	})

	api.Delete("/sessions/:id", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		sessionID := strings.TrimSpace(c.Params("id"))
		if sessionID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "session id is required"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		sess, err := svc.End(c.Context(), tenantID, sessionID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(sess)
	})

	return nil
}
