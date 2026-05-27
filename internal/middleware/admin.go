package middleware

import "github.com/gofiber/fiber/v2"

// RequireAdminRole blocks non-admin API keys from tenant administration.
func RequireAdminRole(c *fiber.Ctx) error {
	role, _ := c.Locals(RoleContextKey).(string)
	if role != "admin" {
		return c.Status(403).JSON(fiber.Map{
			"error": "admin role required",
		})
	}
	return c.Next()
}

// RequireAdminRoleIfAPIKeyPresent enforces admin role when the client sent X-API-Key
// (e.g. programmatic GET /v1/admin/ui). Shell loads without a header pass through.
func RequireAdminRoleIfAPIKeyPresent(c *fiber.Ctx) error {
	if c.Get("X-API-Key") == "" {
		return c.Next()
	}
	return RequireAdminRole(c)
}
