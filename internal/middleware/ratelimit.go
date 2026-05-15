package middleware

import (
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimitMiddleware applies per-API-key rate limiting via Fiber's limiter.
func RateLimitMiddleware() fiber.Handler {
	if os.Getenv("RATE_LIMIT_DISABLED") == "true" || os.Getenv("RATE_LIMIT_DISABLED") == "1" {
		return func(c *fiber.Ctx) error { return c.Next() }
	}

	rpm := 120
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rpm = n
		}
	}

	return limiter.New(limiter.Config{
		Max:        rpm,
		Expiration: time.Minute,
		Next: func(c *fiber.Ctx) bool {
			return IsUnauthenticatedProbe(c.Method(), c.Path())
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			if keyID, ok := c.Locals(APIKeyIDContextKey).(string); ok && keyID != "" {
				return keyID
			}
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded"})
		},
	})
}
