package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/event"
)

// RegisterReadyRoutes registers GET /ready and GET /v1/ready (no API key).
// Returns 200 when PostgreSQL and Redis respond to ping; 503 otherwise.
func RegisterReadyRoutes(app *fiber.App, db *pgxpool.Pool) {
	h := func(c *fiber.Ctx) error {
		return readyResponse(c, db)
	}
	app.Get("/ready", h)
	app.Get("/v1/ready", h)
}

func readyResponse(c *fiber.Ctx, db *pgxpool.Pool) error {
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
		"status":       statusTxt,
		"database_ok":  dbOK,
		"redis_ok":     redisOK,
		"service":      "pcmi-api",
		"version":      "v1.18.0",
	})
}
