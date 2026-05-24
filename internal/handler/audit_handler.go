package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type auditLister interface {
	List(ctx context.Context, tenantID string, page model.PageRequest, since *time.Time) ([]model.AuditEntry, model.PageResponse, error)
	Count(ctx context.Context, tenantID string, since *time.Time) (int, error)
}

type AuditHandler struct {
	repo auditLister
}

func NewAuditHandler(db *pgxpool.Pool) *AuditHandler {
	return &AuditHandler{repo: repository.NewAuditRepository(db)}
}

func (h *AuditHandler) List(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)

	pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var since *time.Time
	if s := c.Query("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid since timestamp (RFC3339)"})
		}
		since = &t
	}

	entries, pageResp, err := h.repo.List(c.Context(), tenantID, model.PageRequest{
		Cursor: pageParams.Cursor,
		Limit:  pageParams.Limit,
	}, since)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	total, err := h.repo.Count(c.Context(), tenantID, since)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"entries":     entries,
		"total":       total,
		"limit":       pageParams.Limit,
		"offset":      0,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}
