package middleware

import "github.com/gofiber/fiber/v2"

// IsUnauthenticatedProbe reports routes that skip API key, rate limit, and audit
// (liveness/readiness/metrics only). Keep list aligned across apikey, ratelimit, audit.
func IsUnauthenticatedProbe(method, path string) bool {
	if method != fiber.MethodGet {
		return false
	}
	switch path {
	case "/health", "/v1/health", "/metrics", "/ready", "/v1/ready":
		return true
	default:
		return false
	}
}
