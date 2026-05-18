package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/version"
)

// dbPinger matches *pgxpool.Pool and pgxmock pools for readiness probes.
type dbPinger interface {
	Ping(context.Context) error
}

// RegisterReadyRoutes registers GET /ready and GET /v1/ready (no API key).
// Returns 200 when PostgreSQL and Redis respond to ping; 503 otherwise.
// db can be *pgxpool.Pool or a mock that implements Ping.
func RegisterReadyRoutes(app *fiber.App, db dbPinger) {
	h := func(c *fiber.Ctx) error {
		return readyResponse(c, db)
	}
	app.Get("/ready", h)
	app.Get("/v1/ready", h)
}

func readyResponse(c *fiber.Ctx, db dbPinger) error {
	ctx := c.Context()
	dbOK := db.Ping(ctx) == nil
	redisOK := event.RedisClient != nil && event.RedisClient.Ping(ctx).Err() == nil
	ok := dbOK && redisOK
	code := 503
	statusTxt := "not_ready"
	if ok {
		code = 200
		statusTxt = "ready"
	}
	return c.Status(code).JSON(fiber.Map{
		"status":      statusTxt,
		"database_ok": dbOK,
		"redis_ok":    redisOK,
		"service":     "pcmi-api",
		"version":     version.Tag,
	})
}
