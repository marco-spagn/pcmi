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

		p := c.Path()
		if c.Method() == fiber.MethodGet && (p == "/health" || p == "/v1/health") {
			return c.Next()
		}

		// Proceed with request
		err := c.Next()

		tenantID, _ := c.Locals(TenantContextKey).(string)
		if tenantID == "" {
			return err
		}

		var apiKeyID any
		if id, ok := c.Locals(APIKeyIDContextKey).(string); ok && id != "" {
			apiKeyID = id
		}

		_, dbErr := m.db.Exec(context.Background(), `
			INSERT INTO audit_log (
				tenant_id, api_key_id, event_type, path, method, status_code,
				request_body, response_body, ip_address, user_agent, created_at
			) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		`,
			tenantID,
			apiKeyID,
			"api_request",
			c.Path(),
			c.Method(),
			c.Response().StatusCode(),
			nil,
			nil,
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
