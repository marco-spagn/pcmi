package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type LineageHandler struct {
	repo *repository.LineageRepository
}

func NewLineageHandler(db *pgxpool.Pool) *LineageHandler {
	return &LineageHandler{repo: repository.NewLineageRepository(db)}
}

func (h *LineageHandler) MemoryLineage(c *fiber.Ctx) error {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path is required"})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	resp, err := h.repo.MemoryLineage(c.Context(), tenantID, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

func (h *LineageHandler) DistilledLineage(c *fiber.Ctx) error {
	idStr := strings.TrimSpace(c.Params("id"))
	if idStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id is required"})
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	resp, err := h.repo.DistilledLineage(c.Context(), tenantID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}
