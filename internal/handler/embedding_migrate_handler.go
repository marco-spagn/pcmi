package handler

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

type EmbeddingMigrateHandler struct {
	db *pgxpool.Pool
}

func NewEmbeddingMigrateHandler(db *pgxpool.Pool) *EmbeddingMigrateHandler {
	return &EmbeddingMigrateHandler{db: db}
}

type migrateEmbeddingsRequest struct {
	PathPrefix     string `json:"path_prefix"`
	TargetModel    string `json:"target_model"`
	EmbeddingSpace string `json:"embedding_space"`
}

func (h *EmbeddingMigrateHandler) Migrate(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	var req migrateEmbeddingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	prefix := strings.TrimSpace(req.PathPrefix)
	if prefix == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path_prefix is required"})
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var space *string
	if s := strings.TrimSpace(req.EmbeddingSpace); s != "" {
		space = &s
	}
	var n int
	err := h.db.QueryRow(ctx,
		`SELECT mark_embeddings_for_migration($1::uuid, $2, $3, $4)`,
		tenantID, prefix, req.TargetModel, space,
	).Scan(&n)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":          "queued",
		"marked_count":    n,
		"path_prefix":     prefix,
		"target_model":    req.TargetModel,
		"embedding_space": req.EmbeddingSpace,
	})
}
