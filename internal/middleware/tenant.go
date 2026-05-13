package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const TenantContextKey = "tenant_id"

func TenantMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantIDStr := c.Get("X-Tenant-ID")
		if tenantIDStr == "" {
			tenantIDStr = c.Query("tenant_id")
		}

		if tenantIDStr == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "Missing X-Tenant-ID header or tenant_id query parameter",
			})
		}

		if _, err := uuid.Parse(tenantIDStr); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid tenant_id format (must be UUID)",
			})
		}

		c.Locals(TenantContextKey, tenantIDStr)
		return c.Next()
	}
}
