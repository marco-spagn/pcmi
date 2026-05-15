package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

type WebhookHandler struct {
	db *pgxpool.Pool
}

func NewWebhookHandler(db *pgxpool.Pool) *WebhookHandler {
	return &WebhookHandler{db: db}
}

type registerWebhookRequest struct {
	URL         string   `json:"url"`
	EventTypes  []string `json:"event_types"`
	Secret      string   `json:"secret"`
}

func (h *WebhookHandler) Register(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	var req registerWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "url is required"})
	}
	types := req.EventTypes
	if types == nil {
		types = []string{}
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var id string
	err := h.db.QueryRow(ctx, `
		INSERT INTO webhook_endpoints (tenant_id, url, event_types, secret)
		VALUES ($1::uuid, $2, $3, NULLIF($4, ''))
		RETURNING id::text`,
		tenantID, req.URL, types, req.Secret,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"id": id, "status": "registered"})
}

func (h *WebhookHandler) List(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	rows, err := h.db.Query(ctx, `
		SELECT id::text, url, event_types, enabled, created_at
		FROM webhook_endpoints
		WHERE tenant_id = $1::uuid
		ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	entries := make([]fiber.Map, 0)
	for rows.Next() {
		var id, url string
		var types []string
		var enabled bool
		var createdAt any
		if err := rows.Scan(&id, &url, &types, &enabled, &createdAt); err != nil {
			continue
		}
		entries = append(entries, fiber.Map{
			"id": id, "url": url, "event_types": types, "enabled": enabled, "created_at": createdAt,
		})
	}
	return c.JSON(fiber.Map{"entries": entries, "total": len(entries)})
}
