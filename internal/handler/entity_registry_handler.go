package handler

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/worker"
)

// EntityRegistryHandler exposes Phase D entity registry and alias proposal endpoints.
type EntityRegistryHandler struct {
	registry *service.EntityRegistryService
	aliases  *service.EntityAliasProposalService
}

func newEntityRegistryHandler(dbWrite, readReplica *pgxpool.Pool, graphClient *graph.GraphClient, cfg *config.Config, registry *service.EntityRegistryService) *EntityRegistryHandler {
	entityRepo := repository.NewEntityRegistryRepository(dbWrite, readReplica)
	proposalRepo := repository.NewEntityAliasProposalRepository(dbWrite, readReplica)
	profiles := repository.NewExtractionRepository(dbWrite, readReplica)
	llm, _ := worker.NewLLMClient(cfg)
	aliasSvc := service.NewEntityAliasProposalService(proposalRepo, entityRepo, registry, profiles, llm, cfg)
	return &EntityRegistryHandler{registry: registry, aliases: aliasSvc}
}

// RegisterEntityRegistryRoutes mounts generic entity evolution endpoints (all datasets).
func RegisterEntityRegistryRoutes(app *fiber.App, dbWrite, readReplica *pgxpool.Pool, graphClient *graph.GraphClient, cfg *config.Config, registry *service.EntityRegistryService) {
	if registry == nil {
		return
	}
	h := newEntityRegistryHandler(dbWrite, readReplica, graphClient, cfg, registry)
	app.Get("/v1/entities/registry", h.ListEntities)
	app.Get("/v1/entities/registry/:kind/*", h.GetEntity)
	app.Post("/v1/entities/registry/aliases", middleware.RequireWriteRole, h.AddAlias)
	app.Get("/v1/graph/entity-alias-proposals", h.ListAliasProposals)
	app.Post("/v1/graph/entity-alias-proposals/generate/:memory_id", middleware.RequireWriteRole, h.GenerateAliasProposals)
	app.Post("/v1/graph/entity-alias-proposals/:id/accept", middleware.RequireWriteRole, h.AcceptAliasProposal)
	app.Post("/v1/graph/entity-alias-proposals/:id/reject", middleware.RequireWriteRole, h.RejectAliasProposal)
}

func (h *EntityRegistryHandler) ListEntities(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	kind := strings.TrimSpace(c.Query("kind"))
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := h.registry.ListEntities(c.Context(), tenantID, kind, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "list entities failed"})
	}
	if rows == nil {
		rows = []model.EntityRegistry{}
	}
	return c.JSON(fiber.Map{"entities": rows, "count": len(rows)})
}

func (h *EntityRegistryHandler) GetEntity(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	kind := strings.TrimSpace(c.Params("kind"))
	key := strings.TrimSpace(c.Params("*"))
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		key = strings.TrimSpace(c.Params("canonical_key"))
	}
	if decoded, err := url.PathUnescape(key); err == nil {
		key = decoded
	}
	key = strings.TrimSpace(key)
	entity, aliases, snaps, err := h.registry.GetEntity(c.Context(), tenantID, kind, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"entity":     entity,
		"aliases":    aliases,
		"snapshots":  snaps,
		"snapshot_n": len(snaps),
	})
}

type addAliasRequest struct {
	Kind         string  `json:"kind"`
	CanonicalKey string  `json:"canonical_key"`
	AliasKey     string  `json:"alias_key"`
	Confidence   float64 `json:"confidence"`
}

func (h *EntityRegistryHandler) AddAlias(c *fiber.Ctx) error {
	var req addAliasRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid json body"})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	entity, err := h.registry.AddManualAlias(c.Context(), tenantID, req.Kind, req.CanonicalKey, req.AliasKey, req.Confidence)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"entity": entity})
}

func (h *EntityRegistryHandler) ListAliasProposals(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		status = model.EntityAliasProposalStatusPending
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := h.aliases.List(c.Context(), tenantID, status, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "list alias proposals failed"})
	}
	if rows == nil {
		rows = []model.EntityAliasProposal{}
	}
	return c.JSON(fiber.Map{"proposals": rows, "count": len(rows), "status": status})
}

func (h *EntityRegistryHandler) GenerateAliasProposals(c *fiber.Ctx) error {
	if !h.aliases.Enabled() {
		return c.Status(503).JSON(fiber.Map{
			"error": "entity alias proposals are disabled",
			"hint":  "set ENTITY_ALIAS_PROPOSALS_ENABLED=true on API and worker",
		})
	}
	memoryID, err := parseMemoryIDParam(c.Params("memory_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	rows, err := h.aliases.GenerateForMemory(c.Context(), tenantID, memoryID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			return c.Status(404).JSON(fiber.Map{"error": msg})
		case strings.Contains(msg, "disabled"), strings.Contains(msg, "not configured"):
			return c.Status(503).JSON(fiber.Map{"error": msg})
		default:
			return c.Status(422).JSON(fiber.Map{"error": msg})
		}
	}
	return c.JSON(fiber.Map{"proposals": rows, "count": len(rows), "source_memory_id": memoryID})
}

func (h *EntityRegistryHandler) AcceptAliasProposal(c *fiber.Ctx) error {
	id, err := parseProposalID(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	prop, err := h.aliases.AcceptProposal(c.Context(), tenantID, id)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not found") {
			return c.Status(404).JSON(fiber.Map{"error": msg})
		}
		if strings.Contains(msg, "not pending") {
			return c.Status(409).JSON(fiber.Map{"error": msg})
		}
		return c.Status(400).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"proposal": prop})
}

func (h *EntityRegistryHandler) RejectAliasProposal(c *fiber.Ctx) error {
	id, err := parseProposalID(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	prop, err := h.aliases.RejectProposal(c.Context(), tenantID, id)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not found") {
			return c.Status(404).JSON(fiber.Map{"error": msg})
		}
		if strings.Contains(msg, "not pending") {
			return c.Status(409).JSON(fiber.Map{"error": msg})
		}
		return c.Status(400).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"proposal": prop})
}
