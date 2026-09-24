package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/service"
)

// UsageHandler serves GET /v1/stats/usage (LLM / embedding token FinOps).
type UsageHandler struct {
	svc *service.UsageService
}

// Get handles GET /v1/stats/usage?from=&to=&group_by=.
func (h *UsageHandler) Get(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	rep, err := h.svc.Report(c.Context(), tenantID, service.UsageQuery{
		From:    c.Query("from"),
		To:      c.Query("to"),
		GroupBy: c.Query("group_by"),
	})
	if errors.Is(err, service.ErrInvalidUsageQuery) {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rep)
}
