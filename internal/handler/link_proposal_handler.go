package handler

import (
	"fmt"
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

// LinkProposalHandler exposes Phase C link proposal review endpoints.
type LinkProposalHandler struct {
	svc *service.LinkProposalService
}

func newLinkProposalHandler(dbWrite, readReplica *pgxpool.Pool, graphClient *graph.GraphClient, cfg *config.Config) *LinkProposalHandler {
	proposals := repository.NewLinkProposalRepository(dbWrite, readReplica)
	profiles := repository.NewExtractionRepository(dbWrite, readReplica)
	links := repository.NewLinksRepository(dbWrite, readReplica)
	llm, _ := worker.NewLLMClient(cfg)
	svc := service.NewLinkProposalService(proposals, profiles, links, graphClient, llm, cfg)
	return &LinkProposalHandler{svc: svc}
}

// RegisterLinkProposalRoutes mounts graph link proposal endpoints.
func RegisterLinkProposalRoutes(app *fiber.App, dbWrite, readReplica *pgxpool.Pool, graphClient *graph.GraphClient, cfg *config.Config) {
	if graphClient == nil {
		return
	}
	h := newLinkProposalHandler(dbWrite, readReplica, graphClient, cfg)
	app.Get("/v1/graph/link-proposals", h.List)
	app.Post("/v1/graph/link-proposals/generate/:memory_id", middleware.RequireWriteRole, h.Generate)
	app.Post("/v1/graph/link-proposals/:id/accept", middleware.RequireWriteRole, h.Accept)
	app.Post("/v1/graph/link-proposals/:id/reject", middleware.RequireWriteRole, h.Reject)
}

func (h *LinkProposalHandler) List(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		status = model.LinkProposalStatusPending
	}
	var sourceID int64
	if raw := strings.TrimSpace(c.Query("source_memory_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "invalid source_memory_id"})
		}
		sourceID = id
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := h.svc.List(c.Context(), tenantID, status, sourceID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "list proposals failed"})
	}
	return c.JSON(fiber.Map{"proposals": rows, "count": len(rows), "status": status})
}

func (h *LinkProposalHandler) Generate(c *fiber.Ctx) error {
	if !h.svc.Enabled() {
		return c.Status(503).JSON(fiber.Map{
			"error": "link proposals are disabled",
			"hint":  "set LINK_PROPOSALS_ENABLED=true on API and worker",
		})
	}
	memoryID, err := parseMemoryIDParam(c.Params("memory_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	rows, err := h.svc.GenerateForMemory(c.Context(), tenantID, memoryID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			return c.Status(404).JSON(fiber.Map{"error": msg})
		case strings.Contains(msg, "not configured"), strings.Contains(msg, "disabled"), strings.Contains(msg, "not available"):
			return c.Status(503).JSON(fiber.Map{"error": msg})
		case strings.Contains(msg, "no successful extraction"), strings.Contains(msg, "no matching"):
			return c.Status(422).JSON(fiber.Map{"error": msg})
		default:
			return c.Status(422).JSON(fiber.Map{"error": msg})
		}
	}
	return c.JSON(fiber.Map{"proposals": rows, "count": len(rows), "source_memory_id": memoryID})
}

func (h *LinkProposalHandler) Accept(c *fiber.Ctx) error {
	id, err := parseProposalID(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	link, prop, err := h.svc.AcceptProposal(c.Context(), tenantID, id)
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
	return c.JSON(fiber.Map{"link": link, "proposal": prop})
}

func (h *LinkProposalHandler) Reject(c *fiber.Ctx) error {
	id, err := parseProposalID(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	prop, err := h.svc.RejectProposal(c.Context(), tenantID, id)
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

func parseProposalID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("id is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid proposal id")
	}
	return id, nil
}
