package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

type SummarizeHandler struct {
	svc *service.SummarizeService
}

func NewSummarizeHandler(db *pgxpool.Pool) *SummarizeHandler {
	return &SummarizeHandler{
		svc: service.NewSummarizeService(repository.NewMemoryRepository(db)),
	}
}

func (h *SummarizeHandler) Post(c *fiber.Ctx) error {
	var req service.SummarizeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	result, err := h.svc.Summarize(c.Context(), &req, tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}
