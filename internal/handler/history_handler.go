package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type pathHistoryLister interface {
	ListPathHistory(ctx context.Context, tenantID, path string, page model.PageRequest) ([]model.MemoryEntry, model.PageResponse, error)
	CountPathHistory(ctx context.Context, tenantID, path string) (int, error)
}

type HistoryHandler struct {
	repo pathHistoryLister
}

func NewHistoryHandler(dbWrite, readReplica *pgxpool.Pool) *HistoryHandler {
	return &HistoryHandler{repo: repository.NewMemoryRepository(dbWrite, readReplica)}
}

// Get returns all versions for a path (GET /v1/memories/history).
func (h *HistoryHandler) Get(c *fiber.Ctx) error {
	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path query parameter is required"})
	}

	tenantID := c.Locals(middleware.TenantContextKey).(string)
	pageParams, err := ParseListPagination(c, model.SortKeyIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	entries, pageResp, err := h.repo.ListPathHistory(c.Context(), tenantID, path, model.PageRequest{
		Cursor: pageParams.Cursor,
		Limit:  pageParams.Limit,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	total, err := h.repo.CountPathHistory(c.Context(), tenantID, path)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"path":        path,
		"entries":     entries,
		"total":       total,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}
