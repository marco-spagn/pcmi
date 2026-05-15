package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

type DistilledHandler struct {
	db *pgxpool.Pool
}

func NewDistilledHandler(db *pgxpool.Pool) *DistilledHandler {
	return &DistilledHandler{db: db}
}

func (h *DistilledHandler) Get(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}

	pathPrefix := c.Query("path_prefix")
	if pathPrefix == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path_prefix is required"})
	}

	limit := c.QueryInt("limit", 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	log.Printf("📡 [DISTILLED] tenant=%s path_prefix=%s", tenantID, pathPrefix)

	rows, err := h.db.Query(context.Background(), `
		SELECT id, path::text, summary, insights, confidence_score, distilled_at, source_entry_ids
		FROM distilled_knowledge
		WHERE tenant_id = $1::uuid
		  AND path <@ $2::ltree
		ORDER BY distilled_at DESC
		LIMIT $3`, tenantID, pathPrefix, limit)
	if err != nil {
		log.Printf("❌ [DISTILLED] query: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	results := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id           int64
			path         string
			summary      string
			insightsRaw  []byte
			confidence   sql.NullFloat64
			distilledAt  time.Time
			sourceIDs    []int64
		)
		if err := rows.Scan(&id, &path, &summary, &insightsRaw, &confidence, &distilledAt, &sourceIDs); err != nil {
			log.Printf("❌ [DISTILLED] scan: %v", err)
			continue
		}

		var insights any = json.RawMessage(insightsRaw)
		if len(insightsRaw) > 0 {
			var arr []any
			if json.Unmarshal(insightsRaw, &arr) == nil {
				insights = arr
			}
		}

		row := map[string]any{
			"id":               id,
			"path":             path,
			"summary":          summary,
			"insights":         insights,
			"distilled_at":     distilledAt.Format(time.RFC3339),
			"source_entry_ids": sourceIDs,
		}
		if confidence.Valid {
			row["confidence_score"] = confidence.Float64
		}
		results = append(results, row)
	}

	return c.JSON(fiber.Map{
		"entries": results,
		"total":   len(results),
		"tenant":  tenantID,
	})
}
