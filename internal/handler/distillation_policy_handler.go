package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

// SetupDistillationPolicyRoutes registers distillation policy CRUD and run history.
func SetupDistillationPolicyRoutes(app *fiber.App, db handlerDB) {
	h := NewDistillationPolicyHandler(db)
	api := app.Group("/v1")
	api.Post("/distillation/policies", middleware.RequireWriteRole, h.Create)
	api.Get("/distillation/policies", h.List)
	api.Patch("/distillation/policies/:id", middleware.RequireWriteRole, h.Patch)
	api.Get("/distillation/runs", h.ListRuns)
}

type DistillationPolicyHandler struct {
	db handlerDB
}

func NewDistillationPolicyHandler(db handlerDB) *DistillationPolicyHandler {
	return &DistillationPolicyHandler{db: db}
}

func (h *DistillationPolicyHandler) Create(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	var req model.CreateDistillationPolicyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if strings.TrimSpace(req.Name) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if strings.TrimSpace(req.PathPrefix) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path_prefix is required"})
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	count := req.CountThreshold
	if count < 1 {
		count = 10
	}
	interval := req.MinIntervalSecs
	if interval < 0 {
		interval = 300
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var p model.DistillationPolicy
	err := h.db.QueryRow(ctx, `
		INSERT INTO distillation_policies (
			tenant_id, name, path_prefix, enabled, count_threshold, min_interval_secs, max_age_secs
		) VALUES ($1::uuid, $2, $3::ltree, $4, $5, $6, $7)
		RETURNING id, tenant_id::text, name, path_prefix::text, enabled,
		          count_threshold, min_interval_secs, max_age_secs, last_triggered_at,
		          created_at, updated_at`,
		tenantID, req.Name, req.PathPrefix, enabled, count, interval, req.MaxAgeSecs,
	).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.PathPrefix, &p.Enabled,
		&p.CountThreshold, &p.MinIntervalSecs, &p.MaxAgeSecs, &p.LastTriggeredAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(p)
}

func (h *DistillationPolicyHandler) List(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	q := `
		SELECT id, tenant_id::text, name, path_prefix::text, enabled,
		       count_threshold, min_interval_secs, max_age_secs, last_triggered_at,
		       created_at, updated_at
		FROM distillation_policies
		WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	clause, clauseArgs, err := repository.KeysetCreatedAtIDClause(pageParams.Cursor, model.SortKeyCreatedAtIDDesc, argN)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, argN)
	args = append(args, repository.FetchLimit(pageParams.Limit))

	rows, err := h.db.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	policies := make([]model.DistillationPolicy, 0)
	for rows.Next() {
		var p model.DistillationPolicy
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Name, &p.PathPrefix, &p.Enabled,
			&p.CountThreshold, &p.MinIntervalSecs, &p.MaxAgeSecs, &p.LastTriggeredAt,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			continue
		}
		policies = append(policies, p)
	}
	trimmed, pageResp, err := model.FinishInt64Page(policies, pageParams.Limit, model.SortKeyCreatedAtIDDesc,
		func(p model.DistillationPolicy) int64 { return p.ID },
		func(p model.DistillationPolicy) time.Time { return p.CreatedAt },
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"policies":    trimmed,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}

func (h *DistillationPolicyHandler) Patch(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id < 1 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid policy id"})
	}
	var req model.PatchDistillationPolicyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var p model.DistillationPolicy
	err = h.db.QueryRow(ctx, `
		UPDATE distillation_policies SET
			name = COALESCE($3, name),
			path_prefix = COALESCE($4::ltree, path_prefix),
			enabled = COALESCE($5, enabled),
			count_threshold = COALESCE($6, count_threshold),
			min_interval_secs = COALESCE($7, min_interval_secs),
			max_age_secs = COALESCE($8, max_age_secs),
			updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2::uuid
		RETURNING id, tenant_id::text, name, path_prefix::text, enabled,
		          count_threshold, min_interval_secs, max_age_secs, last_triggered_at,
		          created_at, updated_at`,
		id, tenantID, req.Name, req.PathPrefix, req.Enabled,
		req.CountThreshold, req.MinIntervalSecs, req.MaxAgeSecs,
	).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.PathPrefix, &p.Enabled,
		&p.CountThreshold, &p.MinIntervalSecs, &p.MaxAgeSecs, &p.LastTriggeredAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "policy not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(p)
}

func (h *DistillationPolicyHandler) ListRuns(c *fiber.Ctx) error {
	tenantID, ok := c.Locals(middleware.TenantContextKey).(string)
	if !ok || tenantID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "tenant context missing"})
	}
	pageParams, err := ParseListPagination(c, model.SortKeyCreatedAtIDDesc, 50)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	var policyID *int64
	if v := strings.TrimSpace(c.Query("policy_id")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid policy_id"})
		}
		policyID = &n
	}
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	baseQ := `
		SELECT id, policy_id, tenant_id::text, path_prefix, status, error_message,
		       source_count, distilled_id, created_at, completed_at
		FROM distillation_runs
		WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	if policyID != nil {
		baseQ += fmt.Sprintf(` AND policy_id = $%d`, argN)
		args = append(args, *policyID)
		argN++
	}
	clause, clauseArgs, err := repository.KeysetCreatedAtIDClause(pageParams.Cursor, model.SortKeyCreatedAtIDDesc, argN)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	baseQ += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	baseQ += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, argN)
	args = append(args, repository.FetchLimit(pageParams.Limit))

	rows, err := h.db.Query(ctx, baseQ, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	runs := make([]model.DistillationRun, 0)
	for rows.Next() {
		var r model.DistillationRun
		var errMsg *string
		if err := rows.Scan(
			&r.ID, &r.PolicyID, &r.TenantID, &r.PathPrefix, &r.Status, &errMsg,
			&r.SourceCount, &r.DistilledID, &r.CreatedAt, &r.CompletedAt,
		); err != nil {
			continue
		}
		r.ErrorMessage = errMsg
		runs = append(runs, r)
	}
	trimmed, pageResp, err := model.FinishInt64Page(runs, pageParams.Limit, model.SortKeyCreatedAtIDDesc,
		func(r model.DistillationRun) int64 { return r.ID },
		func(r model.DistillationRun) time.Time { return r.CreatedAt },
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"runs":        trimmed,
		"limit":       pageParams.Limit,
		"next_cursor": pageResp.NextCursor,
		"has_more":    pageResp.HasMore,
	})
}
