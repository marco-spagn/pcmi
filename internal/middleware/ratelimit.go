package middleware

import (
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimitMiddleware applies per-role, per-API-key rate limiting.
//
// A separate Fiber limiter instance is created for each role so each bucket
// has its own independent Max value:
//
//   - RATE_LIMIT_RPM_READONLY  (default 200) — read-only API keys
//   - RATE_LIMIT_RPM_WRITE     (default 100) — standard write API keys
//   - RATE_LIMIT_RPM_ADMIN     (default  30) — admin API keys (heavy ops)
//   - RATE_LIMIT_RPM           (default 120) — legacy / unrecognised roles
//
// Set RATE_LIMIT_DISABLED=true to bypass all limits (useful in CI / smoke tests).
func RateLimitMiddleware() fiber.Handler {
	if os.Getenv("RATE_LIMIT_DISABLED") == "true" || os.Getenv("RATE_LIMIT_DISABLED") == "1" {
		return func(c *fiber.Ctx) error { return c.Next() }
	}

	readonlyH := newRoleLimiter(envRPM("RATE_LIMIT_RPM_READONLY", 200))
	writeH := newRoleLimiter(envRPM("RATE_LIMIT_RPM_WRITE", 100))
	adminH := newRoleLimiter(envRPM("RATE_LIMIT_RPM_ADMIN", 30))
	fallbackH := newRoleLimiter(envRPM("RATE_LIMIT_RPM", 120))

	return func(c *fiber.Ctx) error {
		if IsUnauthenticatedProbe(c.Method(), c.Path()) {
			return c.Next()
		}
		role, _ := c.Locals(RoleContextKey).(string)
		switch role {
		case "readonly":
			return readonlyH(c)
		case "admin":
			return adminH(c)
		case "write", "user":
			return writeH(c)
		default:
			return fallbackH(c)
		}
	}
}

// newRoleLimiter builds a Fiber limiter handler for the given RPM.
func newRoleLimiter(rpm int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        rpm,
		Expiration: time.Minute,
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

// RoleLimitFor returns the configured RPM for a given role string.
// Useful for health/admin endpoints that want to expose effective limits.
func RoleLimitFor(role string) int {
	switch role {
	case "readonly":
		return envRPM("RATE_LIMIT_RPM_READONLY", 200)
	case "admin":
		return envRPM("RATE_LIMIT_RPM_ADMIN", 30)
	case "write", "user":
		return envRPM("RATE_LIMIT_RPM_WRITE", 100)
	default:
		return envRPM("RATE_LIMIT_RPM", 120)
	}
}

func envRPM(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
