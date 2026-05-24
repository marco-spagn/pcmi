package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type LinksHandler struct {
	repo *repository.LinksRepository
}

func NewLinksHandler(dbWrite, readReplica *pgxpool.Pool) *LinksHandler {
	return &LinksHandler{repo: repository.NewLinksRepository(dbWrite, readReplica)}
}

func (h *LinksHandler) Post(c *fiber.Ctx) error {
	var req model.CreateLinkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	link, err := h.repo.Create(c.Context(), tenantID, req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(link)
}

func (h *LinksHandler) List(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	links, pageResp, err := h.repo.List(c.Context(), tenantID,
		c.Query("from_path"), c.Query("to_path"), c.Query("link_type"),
		model.PageRequest{Cursor: pageParams.Cursor, Limit: pageParams.Limit})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if links == nil {
		links = []model.MemoryLink{}
	}
	return c.JSON(fiber.Map{
		"entries":     links,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}
