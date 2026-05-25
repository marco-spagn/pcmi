package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/handler/adminui"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

func SetupAdminRoutes(app *fiber.App, db repository.AdminQuerier) {
	repo := repository.NewAdminRepository(db)
	svc := service.NewAdminService(repo)

	// Static admin dashboard: browsers cannot send X-API-Key on navigation.
	// The page prompts for ?key= or uses fetch with the key (see adminui.html).
	adminui.Register(app)

	admin := app.Group("/v1/admin", middleware.RequireAdminRole)

	admin.Post("/tenants", func(c *fiber.Ctx) error {
		var req model.TenantCreateRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		t, err := svc.CreateTenant(c.Context(), &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(t)
	})

	admin.Get("/tenants", func(c *fiber.Ctx) error {
		pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtDesc, 100)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if !pageParams.Cursor.IsZero() && pageParams.Cursor.LastID > 0 && pageParams.Cursor.LastTimestamp.IsZero() {
			return c.Status(400).JSON(fiber.Map{"error": "after_id is not supported for tenant listings; use cursor"})
		}
		tenants, pageResp, err := svc.ListTenants(c.Context(), model.PageRequest{
			Cursor: pageParams.Cursor,
			Limit:  pageParams.Limit,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		total, err := svc.CountTenants(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"tenants":     tenants,
			"total":       total,
			"limit":       pageParams.Limit,
			"next_cursor": pageResp.NextCursor,
			"has_more":    pageResp.HasMore,
		})
	})

	admin.Get("/api-keys", func(c *fiber.Ctx) error {
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		if q := c.Query("tenant_id"); q != "" {
			tenantID = q
		}
		pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtDesc, 50)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if !pageParams.Cursor.IsZero() && pageParams.Cursor.LastID > 0 && pageParams.Cursor.LastTimestamp.IsZero() {
			return c.Status(400).JSON(fiber.Map{"error": "after_id is not supported for api key listings; use cursor"})
		}
		keys, pageResp, err := svc.ListAPIKeys(c.Context(), tenantID, model.PageRequest{
			Cursor: pageParams.Cursor,
			Limit:  pageParams.Limit,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"api_keys":    keys,
			"limit":       pageParams.Limit,
			"next_cursor": pageResp.NextCursor,
			"has_more":    pageResp.HasMore,
		})
	})

	admin.Post("/api-keys", func(c *fiber.Ctx) error {
		var req model.APIKeyCreateRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.TenantID == "" {
			req.TenantID = c.Locals(middleware.TenantContextKey).(string)
		}
		if req.Role == "" {
			req.Role = "user"
		}
		resp, err := svc.CreateAPIKey(c.Context(), &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(resp)
	})

	admin.Post("/api-keys/:id/rotate", func(c *fiber.Ctx) error {
		var req model.APIKeyRotateRequest
		_ = c.BodyParser(&req)
		resp, err := svc.RotateAPIKey(c.Context(), c.Params("id"), req.Name, c.Path(), c.Method(), c.IP())
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(resp)
	})

	admin.Delete("/api-keys/:id", func(c *fiber.Ctx) error {
		if err := svc.RevokeAPIKey(c.Context(), c.Params("id")); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})
}
