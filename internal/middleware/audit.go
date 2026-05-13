package middleware

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditMiddleware struct {
	db *pgxpool.Pool
}

func NewAuditMiddleware(db *pgxpool.Pool) *AuditMiddleware {
	return &AuditMiddleware{db: db}
}

func (m *AuditMiddleware) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Proceed with request
		err := c.Next()

		// Log after request
		tenantID := c.Locals(TenantContextKey).(string)
		apiKeyID := c.Locals(APIKeyContextKey).(string) // optional

		_, dbErr := m.db.Exec(context.Background(), `
			INSERT INTO audit_log (
				tenant_id, api_key_id, event_type, path, method, status_code,
				request_body, response_body, ip_address, user_agent, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		`,
			tenantID,
			apiKeyID,
			"api_request",
			c.Path(),
			c.Method(),
			c.Response().StatusCode(),
			nil, // request_body (can be added later)
			nil, // response_body
			c.IP(),
			c.Get("User-Agent"),
		)

		if dbErr != nil {
			log.Printf("⚠️ Audit log failed: %v", dbErr)
		}

		log.Printf("📊 Audit: %s %s [%d] %s", c.Method(), c.Path(), c.Response().StatusCode(), time.Since(start))
		return err
	}
}
