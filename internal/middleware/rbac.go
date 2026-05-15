package middleware

import "github.com/gofiber/fiber/v2"

// RequireWriteRole blocks readonly API keys from mutating endpoints.
func RequireWriteRole(c *fiber.Ctx) error {
	role, _ := c.Locals(RoleContextKey).(string)
	if role == "readonly" {
		return c.Status(403).JSON(fiber.Map{
			"error": "read-only API key cannot perform write operations",
		})
	}
	return c.Next()
}
