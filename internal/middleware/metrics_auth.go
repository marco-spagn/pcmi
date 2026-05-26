package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/log"
)

const metricsBearerPrefix = "Bearer "

// LogMetricsScrapeAuthState logs a startup warning when METRICS_SCRAPE_TOKEN is unset.
func LogMetricsScrapeAuthState(scrapeToken string) {
	if scrapeToken == "" {
		log.Warn("METRICS_SCRAPE_TOKEN not set, GET /metrics is open without authentication")
	}
}

// MetricsScrapeAuth enforces Authorization: Bearer {token} on /metrics when scrapeToken is set.
// When scrapeToken is empty, /metrics remains open (see LogMetricsScrapeAuthState).
func MetricsScrapeAuth(scrapeToken string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if scrapeToken == "" {
			return c.Next()
		}
		auth := c.Get("Authorization")
		if !strings.HasPrefix(auth, metricsBearerPrefix) {
			return c.Status(401).JSON(fiber.Map{"error": "missing or invalid Authorization header"})
		}
		if strings.TrimSpace(auth[len(metricsBearerPrefix):]) != scrapeToken {
			return c.Status(401).JSON(fiber.Map{"error": "invalid metrics scrape token"})
		}
		return c.Next()
	}
}
