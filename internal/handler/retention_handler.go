package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

// RetentionHandler serves /v1/retention-policies and POST /v1/memories/erase.
type RetentionHandler struct {
	svc *service.RetentionService
}

func NewRetentionHandler(db *pgxpool.Pool) *RetentionHandler {
	return &RetentionHandler{svc: service.NewRetentionService(repository.NewRetentionRepository(db))}
}

func retentionError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidRetentionRequest):
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, repository.ErrRetentionPolicyNotFound):
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
}

// List handles GET /v1/retention-policies.
func (h *RetentionHandler) List(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	policies, err := h.svc.List(c.Context(), tenantID)
	if err != nil {
		return retentionError(c, err)
	}
	return c.JSON(fiber.Map{"policies": policies, "total": len(policies)})
}

// Put handles PUT /v1/retention-policies (admin; upsert by path_prefix).
func (h *RetentionHandler) Put(c *fiber.Ctx) error {
	var req model.RetentionPolicyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	p, err := h.svc.Upsert(c.Context(), tenantID, req)
	if err != nil {
		return retentionError(c, err)
	}
	return c.JSON(p)
}

// Delete handles DELETE /v1/retention-policies?path_prefix=... (admin).
// path_prefix must be present; an empty value addresses the tenant-wide policy.
func (h *RetentionHandler) Delete(c *fiber.Ctx) error {
	if !c.Context().QueryArgs().Has("path_prefix") {
		return c.Status(400).JSON(fiber.Map{"error": "path_prefix query parameter is required (empty for the tenant-wide policy)"})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	if err := h.svc.Delete(c.Context(), tenantID, c.Query("path_prefix")); err != nil {
		return retentionError(c, err)
	}
	return c.JSON(fiber.Map{"status": "deleted", "path_prefix": c.Query("path_prefix")})
}

// Erase handles POST /v1/memories/erase (admin): GDPR right to erasure.
func (h *RetentionHandler) Erase(c *fiber.Ctx) error {
	var req model.EraseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	apiKeyID, _ := c.Locals(middleware.APIKeyIDContextKey).(string)
	res, err := h.svc.Erase(c.Context(), tenantID, req, apiKeyID)
	if err != nil {
		return retentionError(c, err)
	}
	return c.JSON(res)
}
