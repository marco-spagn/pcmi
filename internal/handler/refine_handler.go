package handler

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
)

type RefineHandler struct{}

func NewRefineHandler() *RefineHandler {
	return &RefineHandler{}
}

func (h *RefineHandler) Post(c *fiber.Ctx) error {
	var req model.RefineRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	prefix := strings.TrimSpace(req.PathPrefix)
	if prefix == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path_prefix is required"})
	}

	tenantID := c.Locals(middleware.TenantContextKey).(string)
	payload := map[string]any{
		"tenant_id":    tenantID,
		"path_prefix":  prefix,
		"requested_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := event.PublishEvent(event.EventMemoryRefineRequested, payload); err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "failed to queue refine job"})
	}

	return c.JSON(model.RefineResponse{
		Status:     "queued",
		PathPrefix: prefix,
	})
}
