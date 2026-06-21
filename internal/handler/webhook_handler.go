package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/webhook"
)

const sortKeyWebhookCreatedAt = model.SortKeyCreatedAtDesc

type WebhookHandler struct {
	db handlerDB
}

func NewWebhookHandler(db handlerDB) *WebhookHandler {
	return &WebhookHandler{db: db}
}

type registerWebhookRequest struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	Secret     string   `json:"secret"`
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
	if err := webhook.ValidateTargetURL(req.URL); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
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
	pageParams, err := ParseListPagination(c, sortKeyWebhookCreatedAt, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if !pageParams.Cursor.IsZero() && pageParams.Cursor.LastID > 0 && pageParams.Cursor.LastTimestamp.IsZero() {
		return c.Status(400).JSON(fiber.Map{"error": "after_id is not supported for webhook listings; use cursor"})
	}

	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	q := `
		SELECT id::text, url, event_types, enabled, created_at
		FROM webhook_endpoints
		WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	clause, clauseArgs, err := repository.KeysetTimeClause(pageParams.Cursor, sortKeyWebhookCreatedAt, "created_at", argN)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, argN)
	args = append(args, repository.FetchLimit(pageParams.Limit))

	rows, err := h.db.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type whRow struct {
		id, url string
		types   []string
		enabled bool
		created time.Time
	}
	var scanned []whRow
	for rows.Next() {
		var row whRow
		if err := rows.Scan(&row.id, &row.url, &row.types, &row.enabled, &row.created); err != nil {
			continue
		}
		scanned = append(scanned, row)
	}

	trimmed, pageResp, err := model.FinishInt64Page(scanned, pageParams.Limit, sortKeyWebhookCreatedAt,
		func(r whRow) int64 { return 0 },
		func(r whRow) time.Time { return r.created },
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	entries := make([]fiber.Map, 0, len(trimmed))
	for _, row := range trimmed {
		entries = append(entries, fiber.Map{
			"id": row.id, "url": row.url, "event_types": row.types, "enabled": row.enabled, "created_at": row.created,
		})
	}
	return c.JSON(fiber.Map{
		"entries":     entries,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}

func (h *WebhookHandler) DeadLetter(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	pageParams, err := ParseListPagination(c, sortKeyWebhookCreatedAt, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if !pageParams.Cursor.IsZero() && pageParams.Cursor.LastID > 0 && pageParams.Cursor.LastTimestamp.IsZero() {
		return c.Status(400).JSON(fiber.Map{"error": "after_id is not supported for webhook listings; use cursor"})
	}

	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	q := `
		SELECT wd.id::text, wd.endpoint_id::text, wd.event_type, wd.payload,
		       wd.attempts, wd.last_error, wd.created_at
		FROM webhook_deliveries wd
		WHERE wd.tenant_id = $1::uuid AND wd.status = 'dead_letter'`
	args := []any{tenantID}
	argN := 2
	clause, clauseArgs, err := repository.KeysetTimeClause(pageParams.Cursor, sortKeyWebhookCreatedAt, "wd.created_at", argN)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY wd.created_at DESC LIMIT $%d`, argN)
	args = append(args, repository.FetchLimit(pageParams.Limit))

	rows, err := h.db.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type dlRow struct {
		id, endpointID, eventType, lastError string
		payload                              any
		attempts                             int
		created                              time.Time
	}
	var scanned []dlRow
	for rows.Next() {
		var row dlRow
		if err := rows.Scan(&row.id, &row.endpointID, &row.eventType, &row.payload, &row.attempts, &row.lastError, &row.created); err != nil {
			continue
		}
		scanned = append(scanned, row)
	}

	trimmed, pageResp, err := model.FinishInt64Page(scanned, pageParams.Limit, sortKeyWebhookCreatedAt,
		func(r dlRow) int64 { return 0 },
		func(r dlRow) time.Time { return r.created },
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	entries := make([]fiber.Map, 0, len(trimmed))
	for _, row := range trimmed {
		entries = append(entries, fiber.Map{
			"id": row.id, "endpoint_id": row.endpointID, "event_type": row.eventType,
			"payload": row.payload, "attempts": row.attempts, "last_error": row.lastError, "created_at": row.created,
		})
	}
	return c.JSON(fiber.Map{
		"entries":     entries,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}
