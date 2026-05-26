package middleware

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/log"
	"github.com/marco-spagn/pcmi/internal/repository"
)

const (
	// IdempotencyKeyHeader is the client-supplied UUID for POST /v1/memories deduplication.
	IdempotencyKeyHeader = "X-Idempotency-Key"
	// IdempotencyReplayedHeader is set to "true" when a cached response is returned.
	IdempotencyReplayedHeader = "X-Idempotency-Replayed"
)

// IdempotencyCache loads and stores cached store responses.
type IdempotencyCache interface {
	Get(ctx context.Context, tenantID, key string) (json.RawMessage, bool, error)
	Put(ctx context.Context, tenantID, key string, response json.RawMessage) error
}

// Idempotency caches successful POST /v1/memories responses for 24h per tenant + key.
func Idempotency(db *pgxpool.Pool) fiber.Handler {
	return NewIdempotencyMiddleware(repository.NewIdempotencyRepository(db))
}

// NewIdempotencyMiddleware builds the handler with an explicit cache backend (tests).
func NewIdempotencyMiddleware(cache IdempotencyCache) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := strings.TrimSpace(c.Get(IdempotencyKeyHeader))
		if key == "" {
			return c.Next()
		}
		if _, err := uuid.Parse(key); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid X-Idempotency-Key: must be a UUID"})
		}

		tenantID, _ := c.Locals(TenantContextKey).(string)
		if tenantID == "" {
			return c.Next()
		}

		cached, ok, err := cache.Get(c.Context(), tenantID, key)
		if err != nil {
			log.Error("idempotency lookup failed", "err", err)
			return c.Status(500).JSON(fiber.Map{"error": "idempotency lookup failed"})
		}
		if ok {
			c.Set(IdempotencyReplayedHeader, "true")
			return c.Status(fiber.StatusOK).Type("json").Send(cached)
		}

		if err := c.Next(); err != nil {
			return err
		}
		if c.Response().StatusCode() != fiber.StatusOK {
			return nil
		}

		body := c.Response().Body()
		if len(body) == 0 {
			return nil
		}
		if putErr := cache.Put(c.Context(), tenantID, key, body); putErr != nil {
			log.Warn("idempotency cache store failed", "err", putErr)
		}
		return nil
	}
}
