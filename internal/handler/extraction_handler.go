package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/worker"
)

// ExtractionHandler exposes extraction profile and memory extraction endpoints.
type ExtractionHandler struct {
	svc *service.ExtractionService
}

func NewExtractionHandler(dbWrite, readReplica *pgxpool.Pool, cfg *config.Config) *ExtractionHandler {
	profiles := repository.NewExtractionRepository(dbWrite, readReplica)
	memRepo := repository.NewMemoryRepository(dbWrite, readReplica)
	llm, _ := worker.NewLLMClient(cfg)
	svc := service.NewExtractionService(profiles, memRepo, llm, cfg, graph.NewGraphClient(dbWrite))
	return &ExtractionHandler{svc: svc}
}

// RegisterExtractionRoutes mounts Phase A extraction endpoints.
func RegisterExtractionRoutes(app *fiber.App, dbWrite, readReplica *pgxpool.Pool, cfg *config.Config) {
	h := NewExtractionHandler(dbWrite, readReplica, cfg)
	api := app.Group("/v1")
	api.Get("/extraction-profiles", h.ListProfiles)
	api.Put("/extraction-profiles/:profile_id", middleware.RequireWriteRole, h.UpsertProfile)
	api.Delete("/extraction-profiles/:profile_id", middleware.RequireWriteRole, h.DeleteProfile)
	api.Get("/memories/extraction/:memory_id", h.GetExtraction)
	api.Post("/memories/extraction/:memory_id", middleware.RequireWriteRole, h.RunExtraction)
}

func (h *ExtractionHandler) ListProfiles(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	rows, err := h.svc.ListProfiles(c.Context(), tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "list profiles failed"})
	}
	if rows == nil {
		rows = []repository.ExtractionProfileRow{}
	}
	return c.JSON(fiber.Map{"profiles": rows, "count": len(rows)})
}

type upsertProfileRequest struct {
	PathPrefix string             `json:"path_prefix"`
	Enabled    *bool              `json:"enabled"`
	Profile    *extraction.Profile `json:"profile"`
}

func (h *ExtractionHandler) UpsertProfile(c *fiber.Ctx) error {
	profileID := strings.TrimSpace(c.Params("profile_id"))
	if profileID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "profile_id is required"})
	}
	var req upsertProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if req.Profile == nil {
		return c.Status(400).JSON(fiber.Map{"error": "profile body is required"})
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	row, err := h.svc.UpsertProfile(c.Context(), tenantID, profileID, req.PathPrefix, req.Profile, enabled)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(row)
}

func (h *ExtractionHandler) DeleteProfile(c *fiber.Ctx) error {
	profileID := strings.TrimSpace(c.Params("profile_id"))
	if profileID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "profile_id is required"})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	ok, err := h.svc.DeleteProfile(c.Context(), tenantID, profileID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "delete profile failed"})
	}
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "profile not found"})
	}
	return c.SendStatus(204)
}

func (h *ExtractionHandler) GetExtraction(c *fiber.Ctx) error {
	memoryID, err := parseMemoryIDParam(c.Params("memory_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	rec, entry, err := h.svc.GetExtraction(c.Context(), tenantID, memoryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "memory not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "get extraction failed"})
	}
	if rec == nil {
		return c.JSON(fiber.Map{
			"memory_id": memoryID,
			"path":      entry.Path,
			"version":   entry.Version,
			"extraction": nil,
		})
	}
	return c.JSON(fiber.Map{
		"memory_id":  memoryID,
		"path":       entry.Path,
		"version":    entry.Version,
		"extraction": rec,
	})
}

func (h *ExtractionHandler) RunExtraction(c *fiber.Ctx) error {
	if !h.svc.Enabled() {
		return c.Status(503).JSON(fiber.Map{
			"error": "extraction is disabled",
			"hint":  "set EXTRACTION_ENABLED=true on API and worker",
		})
	}
	memoryID, err := parseMemoryIDParam(c.Params("memory_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	rec, err := h.svc.ExtractMemory(c.Context(), tenantID, memoryID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			return c.Status(404).JSON(fiber.Map{"error": "memory not found"})
		case strings.Contains(msg, "no matching"):
			return c.Status(404).JSON(fiber.Map{"error": msg})
		case strings.Contains(msg, "not configured"):
			return c.Status(503).JSON(fiber.Map{"error": msg})
		default:
			if rec != nil {
				return c.Status(422).JSON(fiber.Map{"error": msg, "extraction": rec})
			}
			return c.Status(422).JSON(fiber.Map{"error": msg})
		}
	}
	return c.JSON(fiber.Map{"extraction": rec})
}

func parseMemoryIDParam(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("memory_id is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid memory_id")
	}
	return id, nil
}
