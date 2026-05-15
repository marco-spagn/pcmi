package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

func registerBatchRoutes(api fiber.Router, svc *service.MemoryService) {
	api.Post("/memories/batch", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.BatchStoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.BatchStore(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	api.Post("/retrieve/batch", func(c *fiber.Ctx) error {
		var req model.BatchRetrieveRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.BatchRetrieve(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	api.Post("/memories/export", func(c *fiber.Ctx) error {
		var req model.MemoryExportRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Export(c.Context(), tenantID, &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	api.Post("/memories/import", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.MemoryImportRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Import(c.Context(), tenantID, &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})
}
