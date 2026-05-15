package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	APIKeyContextKey   = "api_key"
	APIKeyIDContextKey = "api_key_id"
	RoleContextKey     = "role"
	TenantContextKey   = "tenant_id"
)

func APIKeyMiddleware(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Path()
		if c.Method() == fiber.MethodGet && (p == "/health" || p == "/v1/health") {
			return c.Next()
		}

		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Missing X-API-Key header"})
		}

		// Calcola hash della chiave
		hash := sha256.Sum256([]byte(apiKey))
		keyHash := hex.EncodeToString(hash[:])

		var apiKeyID, tenantID, role string
		var isActive bool
		err := db.QueryRow(c.Context(), `
			SELECT id::text, tenant_id, role, is_active
			FROM api_keys
			WHERE key_hash = $1
			  AND (expires_at IS NULL OR expires_at > NOW())
		`, keyHash).Scan(&apiKeyID, &tenantID, &role, &isActive)

		if err != nil || !isActive {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid or expired API key"})
		}

		// Imposta tutto nel contesto
		c.Locals(APIKeyContextKey, apiKey)
		c.Locals(APIKeyIDContextKey, apiKeyID)
		c.Locals(RoleContextKey, role)
		c.Locals(TenantContextKey, tenantID)

		// Imposta tenant nel DB per RLS
		_, _ = db.Exec(c.Context(), "SELECT set_tenant_context($1::uuid)", tenantID)

		log.Printf("🔑 API Key authenticated → tenant=%s, role=%s", tenantID, role)
		return c.Next()
	}
}
