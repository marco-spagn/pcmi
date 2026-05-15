package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type AuditHandler struct {
	repo *repository.AuditRepository
}

func NewAuditHandler(db *pgxpool.Pool) *AuditHandler {
	return &AuditHandler{repo: repository.NewAuditRepository(db)}
}

func (h *AuditHandler) List(c *fiber.Ctx) error {
	tenantID := c.Locals(middleware.TenantContextKey).(string)

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	var since *time.Time
	if s := c.Query("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid since timestamp (RFC3339)"})
		}
		since = &t
	}

	entries, total, err := h.repo.List(c.Context(), tenantID, limit, offset, since)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}
