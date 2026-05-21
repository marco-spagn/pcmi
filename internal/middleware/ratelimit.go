package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/marco-spagn/pcmi/internal/config"
)

// RateLimitMiddleware applies per-role, per-API-key rate limiting using cfg.
//
// A separate Fiber limiter instance is created for each role so each bucket
// has its own independent Max value:
//
//   - RateLimitRPMReadonly  (default 200) — read-only API keys
//   - RateLimitRPMWrite     (default 100) — standard write API keys
//   - RateLimitRPMAdmin     (default  30) — admin API keys (heavy ops)
//   - RateLimitRPM          (default 120) — legacy / unrecognised roles
//
// Set RateLimitDisabled=true to bypass all limits (useful in CI / smoke tests).
func RateLimitMiddleware(cfg *config.Config) fiber.Handler {
	if cfg != nil && cfg.RateLimitDisabled {
		return func(c *fiber.Ctx) error { return c.Next() }
	}

	readonlyH := newRoleLimiter(roleRPM(cfg, "readonly"))
	writeH := newRoleLimiter(roleRPM(cfg, "write"))
	adminH := newRoleLimiter(roleRPM(cfg, "admin"))
	fallbackH := newRoleLimiter(roleRPM(cfg, ""))

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
func RoleLimitFor(role string, cfg *config.Config) int {
	return roleRPM(cfg, role)
}

func roleRPM(cfg *config.Config, role string) int {
	if cfg == nil {
		switch role {
		case "readonly":
			return 200
		case "admin":
			return 30
		case "write", "user":
			return 100
		default:
			return 120
		}
	}
	switch role {
	case "readonly":
		if cfg.RateLimitRPMReadonly > 0 {
			return cfg.RateLimitRPMReadonly
		}
		return 200
	case "admin":
		if cfg.RateLimitRPMAdmin > 0 {
			return cfg.RateLimitRPMAdmin
		}
		return 30
	case "write", "user":
		if cfg.RateLimitRPMWrite > 0 {
			return cfg.RateLimitRPMWrite
		}
		return 100
	default:
		if cfg.RateLimitRPM > 0 {
			return cfg.RateLimitRPM
		}
		return 120
	}
}
