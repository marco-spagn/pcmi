package handler

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

type auditLister interface {
	List(ctx context.Context, tenantID string, page model.PageRequest, since *time.Time) ([]model.AuditEntry, model.PageResponse, error)
	Count(ctx context.Context, tenantID string, since *time.Time) (int, error)
}

type AuditHandler struct {
	repo  auditLister
	chain *service.AuditService
}

// NewAuditHandler serves the audit trail. signingKey (AUDIT_EXPORT_SIGNING_KEY)
// may be empty, in which case GET /v1/audit/export is unsigned.
func NewAuditHandler(db *pgxpool.Pool, signingKey string) *AuditHandler {
	repo := repository.NewAuditRepository(db)
	return &AuditHandler{repo: repo, chain: service.NewAuditService(repo, signingKey)}
}

func (h *AuditHandler) List(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)

	pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if !pageParams.Cursor.IsZero() && pageParams.Cursor.LastID > 0 && pageParams.Cursor.LastTimestamp.IsZero() {
		return c.Status(400).JSON(fiber.Map{"error": "after_id is not supported for audit listings; use cursor"})
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

// Verify handles GET /v1/audit/verify: recompute the tenant's hash chain.
func (h *AuditHandler) Verify(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	res, err := h.chain.Verify(c.Context(), tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// Export handles GET /v1/audit/export (admin): JSONL entries + sealed trailer.
func (h *AuditHandler) Export(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)
	var opts service.AuditExportOptions
	for _, q := range []struct {
		name string
		dst  *int64
	}{{"from_seq", &opts.FromSeq}, {"to_seq", &opts.ToSeq}} {
		if raw := c.Query(q.name); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": q.name + " must be an integer"})
			}
			*q.dst = n
		}
	}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "limit must be an integer"})
		}
		opts.Limit = n
	}

	var buf bytes.Buffer
	trailer, err := h.chain.Export(c.Context(), tenantID, opts, &buf)
	if errors.Is(err, service.ErrAuditExportRange) {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set(fiber.HeaderContentType, "application/x-ndjson")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="pcmi-audit-`+tenantID+`.jsonl"`)
	c.Set("X-PCMI-Audit-Head-Hash", trailer.HeadHash)
	c.Set("X-PCMI-Audit-Complete", strconv.FormatBool(trailer.Complete))
	return c.Send(buf.Bytes())
}
