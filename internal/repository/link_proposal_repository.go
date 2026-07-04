package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

// LinkProposalRepository manages graph_link_proposals rows.
type LinkProposalRepository struct {
	w *pgxpool.Pool
	r *pgxpool.Pool
}

func NewLinkProposalRepository(dbWrite, readReplica *pgxpool.Pool) *LinkProposalRepository {
	r := readReplica
	if r == nil {
		r = dbWrite
	}
	return &LinkProposalRepository{w: dbWrite, r: r}
}

func (r *LinkProposalRepository) setTenant(ctx context.Context, tenantID string) error {
	_, err := r.w.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID)
	return err
}

type InsertLinkProposalParams struct {
	SourceMemoryID int64
	FromMemoryID   int64
	ToMemoryID     int64
	FromPath       string
	ToPath         string
	LinkType       string
	Confidence     float64
	Reason         string
	ProfileID      string
	Model          string
	Metadata       map[string]interface{}
}

// InsertPending stores a pending proposal, skipping duplicates and existing links.
func (r *LinkProposalRepository) InsertPending(ctx context.Context, tenantID string, p InsertLinkProposalParams) (*model.LinkProposal, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	meta := p.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	var row model.LinkProposal
	err = r.w.QueryRow(ctx, `
		INSERT INTO graph_link_proposals (
			tenant_id, source_memory_id, from_memory_id, to_memory_id,
			from_path, to_path, link_type, status, confidence, reason,
			profile_id, model, metadata
		)
		SELECT $1::uuid, $2, $3, $4, $5::ltree, $6::ltree, $7, 'pending', $8, $9, $10, $11, $12::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM memory_links ml
			WHERE ml.tenant_id = $1::uuid
			  AND ml.from_path = $5::ltree
			  AND ml.to_path = $6::ltree
			  AND ml.link_type = $7
		)
		ON CONFLICT (tenant_id, from_path, to_path, link_type) WHERE (status = 'pending') DO NOTHING
		RETURNING id, source_memory_id, from_memory_id, to_memory_id,
		          from_path::text, to_path::text, link_type, status,
		          confidence, reason, profile_id, model, metadata, created_at, reviewed_at`,
		tenantID, p.SourceMemoryID, p.FromMemoryID, p.ToMemoryID,
		p.FromPath, p.ToPath, p.LinkType, p.Confidence, p.Reason,
		nullableText(p.ProfileID), nullableText(p.Model), metaJSON,
	).Scan(
		&row.ID, &row.SourceMemoryID, &row.FromMemoryID, &row.ToMemoryID,
		&row.FromPath, &row.ToPath, &row.LinkType, &row.Status,
		&row.Confidence, &row.Reason, &row.ProfileID, &row.Model, &row.Metadata,
		&row.CreatedAt, &row.ReviewedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("insert link proposal: %w", err)
	}
	return &row, nil
}

func nullableText(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// GetByID loads a proposal for the tenant.
func (r *LinkProposalRepository) GetByID(ctx context.Context, tenantID string, id int64) (*model.LinkProposal, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var row model.LinkProposal
	err := r.r.QueryRow(ctx, `
		SELECT id, source_memory_id, from_memory_id, to_memory_id,
		       from_path::text, to_path::text, link_type, status,
		       confidence, reason, COALESCE(profile_id, ''), COALESCE(model, ''),
		       metadata, created_at, reviewed_at
		FROM graph_link_proposals
		WHERE tenant_id = $1::uuid AND id = $2`,
		tenantID, id,
	).Scan(
		&row.ID, &row.SourceMemoryID, &row.FromMemoryID, &row.ToMemoryID,
		&row.FromPath, &row.ToPath, &row.LinkType, &row.Status,
		&row.Confidence, &row.Reason, &row.ProfileID, &row.Model, &row.Metadata,
		&row.CreatedAt, &row.ReviewedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get link proposal: %w", err)
	}
	return &row, nil
}

// List returns proposals filtered by status and optional source memory id.
func (r *LinkProposalRepository) List(ctx context.Context, tenantID, status string, sourceMemoryID int64, limit int) ([]model.LinkProposal, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = model.LinkProposalStatusPending
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}

	q := `
		SELECT id, source_memory_id, from_memory_id, to_memory_id,
		       from_path::text, to_path::text, link_type, status,
		       confidence, reason, COALESCE(profile_id, ''), COALESCE(model, ''),
		       metadata, created_at, reviewed_at
		FROM graph_link_proposals
		WHERE tenant_id = $1::uuid AND status = $2`
	args := []any{tenantID, status}
	if sourceMemoryID > 0 {
		q += ` AND source_memory_id = $3`
		args = append(args, sourceMemoryID)
		q += ` ORDER BY created_at DESC, id DESC LIMIT $4`
		args = append(args, limit)
	} else {
		q += ` ORDER BY created_at DESC, id DESC LIMIT $3`
		args = append(args, limit)
	}

	rows, err := r.r.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list link proposals: %w", err)
	}
	defer rows.Close()

	var out []model.LinkProposal
	for rows.Next() {
		var row model.LinkProposal
		if err := rows.Scan(
			&row.ID, &row.SourceMemoryID, &row.FromMemoryID, &row.ToMemoryID,
			&row.FromPath, &row.ToPath, &row.LinkType, &row.Status,
			&row.Confidence, &row.Reason, &row.ProfileID, &row.Model, &row.Metadata,
			&row.CreatedAt, &row.ReviewedAt,
		); err != nil {
			return nil, fmt.Errorf("scan link proposal: %w", err)
		}
		out = append(out, row)
	}
	if out == nil {
		out = []model.LinkProposal{}
	}
	return out, rows.Err()
}

// UpdateStatus transitions a proposal to accepted or rejected.
func (r *LinkProposalRepository) UpdateStatus(ctx context.Context, tenantID string, id int64, status string) (*model.LinkProposal, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var row model.LinkProposal
	err := r.w.QueryRow(ctx, `
		UPDATE graph_link_proposals
		SET status = $3, reviewed_at = NOW()
		WHERE tenant_id = $1::uuid AND id = $2 AND status = 'pending'
		RETURNING id, source_memory_id, from_memory_id, to_memory_id,
		          from_path::text, to_path::text, link_type, status,
		          confidence, reason, COALESCE(profile_id, ''), COALESCE(model, ''),
		          metadata, created_at, reviewed_at`,
		tenantID, id, status,
	).Scan(
		&row.ID, &row.SourceMemoryID, &row.FromMemoryID, &row.ToMemoryID,
		&row.FromPath, &row.ToPath, &row.LinkType, &row.Status,
		&row.Confidence, &row.Reason, &row.ProfileID, &row.Model, &row.Metadata,
		&row.CreatedAt, &row.ReviewedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update link proposal status: %w", err)
	}
	return &row, nil
}

// LinkExists reports whether memory_links already has the edge.
func (r *LinkProposalRepository) LinkExists(ctx context.Context, tenantID, fromPath, toPath, linkType string) (bool, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return false, err
	}
	var exists bool
	err := r.r.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM memory_links
			WHERE tenant_id = $1::uuid
			  AND from_path = $2::ltree
			  AND to_path = $3::ltree
			  AND link_type = $4
		)`, tenantID, fromPath, toPath, linkType).Scan(&exists)
	return exists, err
}
