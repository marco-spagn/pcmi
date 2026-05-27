package middleware

import "github.com/gofiber/fiber/v2"

// IsUnauthenticatedProbe reports routes that skip API key, rate limit, and audit
// (liveness/readiness/metrics only). Keep list aligned across apikey, ratelimit, audit.
// GET /metrics may still require Authorization: Bearer when METRICS_SCRAPE_TOKEN is set
// (see MetricsScrapeAuth).
func IsUnauthenticatedProbe(method, path string) bool {
	if method != fiber.MethodGet {
		return false
	}
	switch path {
	case "/health", "/v1/health", "/metrics", "/ready", "/v1/ready", "/v1/graph/health", "/v1/graph/ui":
		return true
	default:
		return false
	}
}

// IsAdminUIShellGET is GET /v1/admin/ui without X-API-Key (browser HTML shell).
func IsAdminUIShellGET(method, path string, hasAPIKey bool) bool {
	return method == fiber.MethodGet && path == "/v1/admin/ui" && !hasAPIKey
}
