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
	ListPathHistory(ctx context.Context, tenantID, path string, limit int) ([]model.MemoryEntry, error)
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
	limit := c.QueryInt("limit", 50)

	entries, err := h.repo.ListPathHistory(c.Context(), tenantID, path, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"path":    path,
		"entries": entries,
		"total":   len(entries),
	})
}
