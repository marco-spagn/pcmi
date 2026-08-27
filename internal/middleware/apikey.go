package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// apiKeyDB is satisfied by *pgxpool.Pool and pgxmock pools in tests.
type apiKeyDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

const (
	APIKeyContextKey   = "api_key"
	APIKeyIDContextKey = "api_key_id"
	RoleContextKey     = "role"
	TenantContextKey   = "tenant_id"
)

// APIKeyMiddleware authenticates requests with X-API-Key only (no SSO).
func APIKeyMiddleware(db *pgxpool.Pool) fiber.Handler {
	return authMiddleware(db, nil)
}

// AuthMiddleware authenticates requests with either an OIDC `Authorization:
// Bearer <jwt>` (when oidc != nil) or an X-API-Key. Bearer takes precedence
// when present; otherwise the request falls through to the API-key path, so
// existing clients are unaffected.
func AuthMiddleware(db *pgxpool.Pool, oidc *OIDCVerifier) fiber.Handler {
	return authMiddleware(db, oidc)
}

// apiKeyMiddleware is the X-API-Key-only path (no OIDC). Retained for tests and
// internal callers that predate SSO.
func apiKeyMiddleware(db apiKeyDB) fiber.Handler {
	return authMiddleware(db, nil)
}

func authMiddleware(db apiKeyDB, oidc *OIDCVerifier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if IsUnauthenticatedProbe(c.Method(), c.Path()) {
			return c.Next()
		}

		// SSO/OIDC bearer path — additive, only when configured and present.
		if oidc != nil {
			if bearer := bearerToken(c); bearer != "" {
				return authenticateBearer(c, db, oidc, bearer)
			}
		}

		apiKey := c.Get("X-API-Key")
		if IsAdminUIShellGET(c.Method(), c.Path(), apiKey != "") {
			return c.Next()
		}
		if apiKey == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Missing X-API-Key header"})
		}

		// Compute key hash
		hash := sha256.Sum256([]byte(apiKey))
		keyHash := hex.EncodeToString(hash[:])

		var apiKeyID, tenantID, role string
		var isActive bool
		err := db.QueryRow(c.Context(), `
			SELECT id::text, tenant_id::text, role, is_active
			FROM api_keys
			WHERE key_hash = $1
			  AND is_active = true
			  AND (expires_at IS NULL OR expires_at > NOW())
			  AND (
			        rotated_to IS NULL
			        OR (rotation_grace_ends_at IS NOT NULL AND rotation_grace_ends_at > NOW())
			      )
		`, keyHash).Scan(&apiKeyID, &tenantID, &role, &isActive)

		if err != nil || !isActive {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid or expired API key"})
		}

		// Set everything in the context
		c.Locals(APIKeyContextKey, apiKey)
		c.Locals(APIKeyIDContextKey, apiKeyID)
		c.Locals(RoleContextKey, role)
		c.Locals(TenantContextKey, tenantID)

		// BUG-FIX-5: set_tenant_context activates PostgreSQL RLS for this
		// connection. Silently ignoring the error (_, _ = ...) means that on
		// a DB hiccup the request proceeds without tenant isolation, risking
		// cross-tenant data exposure. Return 503 instead.
		if _, err := db.Exec(c.Context(), "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
			log.Printf("set_tenant_context failed for tenant=%s: %v", tenantID, err)
			return c.Status(503).JSON(fiber.Map{"error": "tenant context unavailable, please retry"})
		}

		touchAPIKeyLastUsed(db, apiKeyID, c.IP())

		log.Printf("API Key authenticated → tenant=%s, role=%s", tenantID, role)
		return c.Next()
	}
}

// bearerToken extracts the token from an `Authorization: Bearer <token>` header.
func bearerToken(c *fiber.Ctx) string {
	h := c.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// authenticateBearer verifies an OIDC bearer token, confirms the token's tenant
// exists, and sets the same request context as the API-key path (role, tenant,
// RLS). Generic 401s avoid leaking whether the token or the tenant was at fault.
func authenticateBearer(c *fiber.Ctx, db apiKeyDB, oidc *OIDCVerifier, bearer string) error {
	tenantID, role, err := oidc.Authenticate(c.Context(), bearer)
	if err != nil {
		log.Printf("OIDC auth rejected: %v", err)
		return c.Status(401).JSON(fiber.Map{"error": "invalid bearer token"})
	}

	// The tenant claim must reference a real tenant. The ::uuid cast also
	// rejects a malformed tenant id before it reaches set_tenant_context.
	var exists bool
	if err := db.QueryRow(c.Context(),
		`SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1::uuid)`, tenantID,
	).Scan(&exists); err != nil || !exists {
		return c.Status(401).JSON(fiber.Map{"error": "unknown tenant"})
	}

	c.Locals(RoleContextKey, role)
	c.Locals(TenantContextKey, tenantID)

	if _, err := db.Exec(c.Context(), "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		log.Printf("set_tenant_context failed for tenant=%s: %v", tenantID, err)
		return c.Status(503).JSON(fiber.Map{"error": "tenant context unavailable, please retry"})
	}

	log.Printf("OIDC authenticated → tenant=%s, role=%s", tenantID, role)
	return c.Next()
}

func touchAPIKeyLastUsed(db apiKeyDB, keyID, clientIP string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = db.Exec(ctx,
			`UPDATE api_keys SET last_used_at = NOW(), last_used_ip = $2 WHERE id = $1::uuid`,
			keyID, clientIP,
		)
	}()
}
