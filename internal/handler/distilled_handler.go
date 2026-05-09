package handler

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DistilledHandler struct {
	db *pgxpool.Pool
}

func NewDistilledHandler(db *pgxpool.Pool) *DistilledHandler {
	return &DistilledHandler{db: db}
}

func (h *DistilledHandler) Get(c *fiber.Ctx) error {
	tenantID := c.Query("tenant_id", "00000000-0000-0000-0000-000000000000")
	pathPrefix := c.Query("path_prefix", "root.test")

	log.Printf("📡 [DISTILLED] GET /v1/distilled chiamato - tenant=%s, path_prefix=%s", tenantID, pathPrefix)

	rows, err := h.db.Query(context.Background(), `
		SELECT id, path, summary, insights::text, confidence_score, distilled_at
		FROM distilled_knowledge 
		WHERE tenant_id = $1 
		  AND path::text LIKE $2 || '%'
		ORDER BY distilled_at DESC 
		LIMIT 10`, tenantID, pathPrefix)
	if err != nil {
		log.Printf("❌ [DISTILLED] Query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var results []map[string]any
	count := 0
	for rows.Next() {
		count++
		var r struct {
			ID              int64
			Path            string
			Summary         string
			Insights        string
			ConfidenceScore float64
			DistilledAt     time.Time   // <<< FIX: time.Time invece di string
		}
		if err := rows.Scan(&r.ID, &r.Path, &r.Summary, &r.Insights, &r.ConfidenceScore, &r.DistilledAt); err != nil {
			log.Printf("❌ [DISTILLED] Scan error on row %d: %v", count, err)
			continue
		}

		results = append(results, map[string]any{
			"id":               r.ID,
			"path":             r.Path,
			"summary":          r.Summary,
			"insights":         r.Insights,
			"confidence_score": r.ConfidenceScore,
			"distilled_at":     r.DistilledAt.Format(time.RFC3339), // ISO format per JSON
		})
	}

	log.Printf("✅ [DISTILLED] Restituiti %d record distillati per path_prefix=%s", len(results), pathPrefix)

	return c.JSON(fiber.Map{
		"entries": results,
		"total":   len(results),
	})
}
