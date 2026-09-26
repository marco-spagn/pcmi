package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

// ErrRetentionPolicyNotFound is returned when deleting a missing policy.
var ErrRetentionPolicyNotFound = errors.New("retention policy not found")

// RetentionRepository manages retention_policies and GDPR erasure (migration 028).
type RetentionRepository struct {
	db *pgxpool.Pool
}

func NewRetentionRepository(db *pgxpool.Pool) *RetentionRepository {
	return &RetentionRepository{db: db}
}

const retentionPolicyColumns = `id, tenant_id::text, path_prefix::text, superseded_retention_days,
	max_age_days, description, created_at, updated_at`

func scanRetentionPolicy(row pgx.Row) (model.RetentionPolicy, error) {
	var p model.RetentionPolicy
	err := row.Scan(&p.ID, &p.TenantID, &p.PathPrefix, &p.SupersededRetentionDays,
		&p.MaxAgeDays, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// List returns the tenant's policies ordered by prefix.
func (r *RetentionRepository) List(ctx context.Context, tenantID string) ([]model.RetentionPolicy, error) {
	rows, err := r.db.Query(ctx, `SELECT `+retentionPolicyColumns+`
		FROM retention_policies WHERE tenant_id = $1::uuid ORDER BY path_prefix`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("retention list: %w", err)
	}
	defer rows.Close()
	out := []model.RetentionPolicy{}
	for rows.Next() {
		p, err := scanRetentionPolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("retention scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Upsert creates or replaces the policy for (tenant, path_prefix).
func (r *RetentionRepository) Upsert(ctx context.Context, tenantID string, req model.RetentionPolicyRequest) (model.RetentionPolicy, error) {
	p, err := scanRetentionPolicy(r.db.QueryRow(ctx, `
		INSERT INTO retention_policies (tenant_id, path_prefix, superseded_retention_days, max_age_days, description)
		VALUES ($1::uuid, $2::ltree, $3, $4, $5)
		ON CONFLICT (tenant_id, path_prefix) DO UPDATE SET
			superseded_retention_days = EXCLUDED.superseded_retention_days,
			max_age_days              = EXCLUDED.max_age_days,
			description               = EXCLUDED.description,
			updated_at                = NOW()
		RETURNING `+retentionPolicyColumns,
		tenantID, req.PathPrefix, req.SupersededRetentionDays, req.MaxAgeDays, req.Description))
	if err != nil {
		return model.RetentionPolicy{}, fmt.Errorf("retention upsert: %w", err)
	}
	return p, nil
}

// Delete removes the policy for (tenant, path_prefix).
func (r *RetentionRepository) Delete(ctx context.Context, tenantID, pathPrefix string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM retention_policies
		WHERE tenant_id = $1::uuid AND path_prefix = $2::ltree`, tenantID, pathPrefix)
	if err != nil {
		return fmt.Errorf("retention delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRetentionPolicyNotFound
	}
	return nil
}

// Erase hard-deletes every version of every memory under pathPrefix for the
// tenant, plus data derived from them (links, distilled knowledge, link and
// alias proposals, entity snapshots, consolidation runs), in one transaction.
// A non-dry-run erasure also appends a gdpr_erase row to the audit chain.
// dryRun executes the same statements and rolls back, so counts are exact.
// It returns the counts and the AGE vertex ids of the erased memories (path
// and "memory.<id>") for best-effort graph cleanup after commit.
func (r *RetentionRepository) Erase(ctx context.Context, tenantID string, req model.EraseRequest, apiKeyID string) (model.EraseCounts, []string, error) {
	var counts model.EraseCounts
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return counts, nil, fmt.Errorf("erase begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE pcmi_erased ON COMMIT DROP AS
		SELECT id, path::text AS path FROM memory_entries
		WHERE tenant_id = $1::uuid AND path <@ $2::ltree`, tenantID, req.PathPrefix); err != nil {
		return counts, nil, fmt.Errorf("erase collect: %w", err)
	}

	// Statements reference $1 = tenant and, when they contain "$2", $2 = prefix.
	steps := []struct {
		dst *int64
		sql string
	}{
		{&counts.Links, `DELETE FROM memory_links
			WHERE tenant_id = $1::uuid AND (from_path <@ $2::ltree OR to_path <@ $2::ltree)`},
		{&counts.Distilled, `DELETE FROM distilled_knowledge
			WHERE tenant_id = $1::uuid
			  AND (path <@ $2::ltree OR source_entry_ids && ARRAY(SELECT id FROM pcmi_erased))`},
		{&counts.LinkProposals, `DELETE FROM graph_link_proposals
			WHERE tenant_id = $1::uuid
			  AND (from_path <@ $2::ltree OR to_path <@ $2::ltree
			       OR source_memory_id IN (SELECT id FROM pcmi_erased)
			       OR from_memory_id IN (SELECT id FROM pcmi_erased)
			       OR to_memory_id IN (SELECT id FROM pcmi_erased))`},
		{&counts.EntitySnapshots, `DELETE FROM entity_snapshots
			WHERE tenant_id = $1::uuid AND memory_id IN (SELECT id FROM pcmi_erased)`},
		{&counts.AliasProposals, `DELETE FROM entity_alias_proposals
			WHERE tenant_id = $1::uuid AND source_memory_id IN (SELECT id FROM pcmi_erased)`},
		{&counts.ConsolidationRuns, `DELETE FROM consolidation_runs
			WHERE tenant_id = $1::uuid
			  AND (source_entry_ids && ARRAY(SELECT id FROM pcmi_erased)
			       OR consolidated_entry_id IN (SELECT id FROM pcmi_erased)
			       OR consolidated_path = $2::text
			       OR left(consolidated_path, length($2::text) + 1) = $2::text || '.')`},
		// Path-based (not id-based) so rows written after the collect step are erased too.
		{&counts.MemoryVersions, `DELETE FROM memory_entries
			WHERE tenant_id = $1::uuid AND path <@ $2::ltree`},
	}
	for _, s := range steps {
		args := []any{tenantID}
		if strings.Contains(s.sql, "$2") {
			args = append(args, req.PathPrefix)
		}
		tag, err := tx.Exec(ctx, s.sql, args...)
		if err != nil {
			return counts, nil, fmt.Errorf("erase: %w", err)
		}
		*s.dst = tag.RowsAffected()
	}
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT path) FROM pcmi_erased`).Scan(&counts.Paths); err != nil {
		return counts, nil, fmt.Errorf("erase paths: %w", err)
	}

	if req.DryRun {
		return counts, nil, nil
	}

	var vertexIDs []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(v), ARRAY[]::text[]) FROM (
			SELECT DISTINCT path AS v FROM pcmi_erased
			UNION
			SELECT 'memory.' || id FROM pcmi_erased
		) s`).Scan(&vertexIDs); err != nil {
		return counts, nil, fmt.Errorf("erase vertex ids: %w", err)
	}

	evidence, err := json.Marshal(map[string]any{
		"path_prefix": req.PathPrefix,
		"reason":      req.Reason,
		"counts":      counts,
	})
	if err != nil {
		return counts, nil, err
	}
	var keyID any
	if apiKeyID != "" {
		keyID = apiKeyID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (tenant_id, api_key_id, event_type, path, method, status_code, request_body, user_agent)
		VALUES ($1::uuid, $2::uuid, 'gdpr_erase', '/v1/memories/erase', 'POST', 200, $3::jsonb, 'pcmi-erase')`,
		tenantID, keyID, string(evidence)); err != nil {
		return counts, nil, fmt.Errorf("erase audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return counts, nil, fmt.Errorf("erase commit: %w", err)
	}
	return counts, vertexIDs, nil
}

// EraseGraphVertices removes AGE :Memory vertices for erased memories when the
// optional graph helper from migration 028 exists. It reports the number of
// vertex ids processed; (0, nil) when AGE is not installed.
func (r *RetentionRepository) EraseGraphVertices(ctx context.Context, tenantID string, vertexIDs []string) (int, error) {
	if len(vertexIDs) == 0 {
		return 0, nil
	}
	var fn *string
	if err := r.db.QueryRow(ctx, `SELECT to_regproc('public.erase_memory_graph_vertices')::text`).Scan(&fn); err != nil {
		return 0, fmt.Errorf("graph erase probe: %w", err)
	}
	if fn == nil {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var n int
	if err := r.db.QueryRow(ctx, `SELECT public.erase_memory_graph_vertices($1::uuid, $2::text[])`,
		tenantID, vertexIDs).Scan(&n); err != nil {
		return 0, fmt.Errorf("graph erase: %w", err)
	}
	return n, nil
}
