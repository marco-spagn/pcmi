package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

// newTestApp builds a minimal Fiber app that injects tenant/role locals
// so handler tests don't need a real DB-backed API-key middleware.
func newTestApp(tenantID, role string) *fiber.App {
	app := fiber.New(fiber.Config{
		// Suppress stack traces in test output.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		c.Locals(middleware.RoleContextKey, role)
		return c.Next()
	})
	return app
}
