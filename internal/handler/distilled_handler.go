package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type distilledQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type DistilledHandler struct {
	db distilledQuerier
}

func NewDistilledHandler(db distilledQuerier) *DistilledHandler {
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

	pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("📡 [DISTILLED] tenant=%s path_prefix=%s", tenantID, pathPrefix)

	q := `
		SELECT id, path::text, summary, insights, confidence_score, distilled_at, source_entry_ids, version
		FROM distilled_knowledge
		WHERE tenant_id = $1::uuid
		  AND path <@ $2::ltree`
	args := []any{tenantID, pathPrefix}
	argN := 3
	clause, clauseArgs, err := repository.KeysetTimeIDClause(pageParams.Cursor, model.SortKeyCreatedAtIDDesc, "distilled_at", argN)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY distilled_at DESC, id DESC LIMIT $%d`, argN)
	args = append(args, repository.FetchLimit(pageParams.Limit))

	rows, err := h.db.Query(context.Background(), q, args...)
	if err != nil {
		log.Printf("❌ [DISTILLED] query: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type distilledRow struct {
		id          int64
		path        string
		summary     string
		insightsRaw []byte
		confidence  sql.NullFloat64
		distilledAt time.Time
		sourceIDs   []int64
		version     int
	}
	var scanned []distilledRow
	for rows.Next() {
		var row distilledRow
		if err := rows.Scan(&row.id, &row.path, &row.summary, &row.insightsRaw, &row.confidence, &row.distilledAt, &row.sourceIDs, &row.version); err != nil {
			log.Printf("❌ [DISTILLED] scan: %v", err)
			continue
		}
		scanned = append(scanned, row)
	}

	trimmed, pageResp, err := model.FinishInt64Page(scanned, pageParams.Limit, model.SortKeyCreatedAtIDDesc,
		func(r distilledRow) int64 { return r.id },
		func(r distilledRow) time.Time { return r.distilledAt },
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	results := make([]map[string]any, 0, len(trimmed))
	for _, row := range trimmed {
		var insights any = json.RawMessage(row.insightsRaw)
		if len(row.insightsRaw) > 0 {
			var arr []any
			if json.Unmarshal(row.insightsRaw, &arr) == nil {
				insights = arr
			}
		}
		out := map[string]any{
			"id":               row.id,
			"path":             row.path,
			"summary":          row.summary,
			"insights":         insights,
			"distilled_at":     row.distilledAt.Format(time.RFC3339),
			"source_entry_ids": row.sourceIDs,
			"version":          row.version,
		}
		if row.confidence.Valid {
			out["confidence_score"] = row.confidence.Float64
		}
		results = append(results, out)
	}

	return c.JSON(fiber.Map{
		"entries":     results,
		"total":       len(results),
		"tenant":      tenantID,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}
