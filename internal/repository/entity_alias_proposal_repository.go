package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

// EntityAliasProposalRepository manages entity_alias_proposals rows.
type EntityAliasProposalRepository struct {
	w *pgxpool.Pool
	r *pgxpool.Pool
}

func NewEntityAliasProposalRepository(dbWrite, readReplica *pgxpool.Pool) *EntityAliasProposalRepository {
	r := readReplica
	if r == nil {
		r = dbWrite
	}
	return &EntityAliasProposalRepository{w: dbWrite, r: r}
}

func (r *EntityAliasProposalRepository) setTenant(ctx context.Context, tenantID string) error {
	_, err := r.w.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID)
	return err
}

type InsertEntityAliasProposalParams struct {
	Kind           string
	AliasKey       string
	SourceEntityID string
	TargetEntityID string
	SourceMemoryID int64
	Confidence     float64
	Reason         string
	Model          string
	Metadata       map[string]interface{}
}

func (r *EntityAliasProposalRepository) InsertPending(ctx context.Context, tenantID string, p InsertEntityAliasProposalParams) (*model.EntityAliasProposal, error) {
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
	var row model.EntityAliasProposal
	var reviewed *string
	err = r.w.QueryRow(ctx, `
		INSERT INTO entity_alias_proposals (
			tenant_id, kind, alias_key, source_entity_id, target_entity_id,
			source_memory_id, status, confidence, reason, model, metadata
		) VALUES ($1::uuid, $2, $3, $4::uuid, $5::uuid, NULLIF($6, 0), 'pending', $7, $8, NULLIF($9, ''), $10::jsonb)
		ON CONFLICT (tenant_id, kind, alias_key, target_entity_id) WHERE (status = 'pending') DO NOTHING
		RETURNING id, kind, alias_key, source_entity_id::text, target_entity_id::text,
		          COALESCE(source_memory_id, 0), status, confidence, reason,
		          COALESCE(model, ''), metadata, created_at, reviewed_at
	`, tenantID, p.Kind, p.AliasKey, p.SourceEntityID, p.TargetEntityID, p.SourceMemoryID,
		p.Confidence, p.Reason, p.Model, metaJSON).Scan(
		&row.ID, &row.Kind, &row.AliasKey, &row.SourceEntityID, &row.TargetEntityID,
		&row.SourceMemoryID, &row.Status, &row.Confidence, &row.Reason, &row.Model,
		&metaJSON, &row.CreatedAt, &reviewed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(metaJSON, &row.Metadata)
	return &row, nil
}

func (r *EntityAliasProposalRepository) GetByID(ctx context.Context, tenantID string, id int64) (*model.EntityAliasProposal, error) {
	var row model.EntityAliasProposal
	var meta []byte
	var reviewed *string
	err := r.r.QueryRow(ctx, `
		SELECT id, kind, alias_key, source_entity_id::text, target_entity_id::text,
		       COALESCE(source_memory_id, 0), status, confidence, reason,
		       COALESCE(model, ''), metadata, created_at, reviewed_at
		FROM entity_alias_proposals
		WHERE tenant_id = $1::uuid AND id = $2
	`, tenantID, id).Scan(
		&row.ID, &row.Kind, &row.AliasKey, &row.SourceEntityID, &row.TargetEntityID,
		&row.SourceMemoryID, &row.Status, &row.Confidence, &row.Reason, &row.Model,
		&meta, &row.CreatedAt, &reviewed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("entity alias proposal not found")
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(meta, &row.Metadata)
	return &row, nil
}

func (r *EntityAliasProposalRepository) List(ctx context.Context, tenantID, status string, limit int) ([]model.EntityAliasProposal, error) {
	if limit <= 0 {
		limit = 50
	}
	if status == "" {
		status = model.EntityAliasProposalStatusPending
	}
	rows, err := r.r.Query(ctx, `
		SELECT id, kind, alias_key, source_entity_id::text, target_entity_id::text,
		       COALESCE(source_memory_id, 0), status, confidence, reason,
		       COALESCE(model, ''), metadata, created_at, reviewed_at
		FROM entity_alias_proposals
		WHERE tenant_id = $1::uuid AND status = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EntityAliasProposal
	for rows.Next() {
		var row model.EntityAliasProposal
		var meta []byte
		var reviewed *string
		if err := rows.Scan(
			&row.ID, &row.Kind, &row.AliasKey, &row.SourceEntityID, &row.TargetEntityID,
			&row.SourceMemoryID, &row.Status, &row.Confidence, &row.Reason, &row.Model,
			&meta, &row.CreatedAt, &reviewed,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meta, &row.Metadata)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *EntityAliasProposalRepository) UpdateStatus(ctx context.Context, tenantID string, id int64, status string) (*model.EntityAliasProposal, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var row model.EntityAliasProposal
	var meta []byte
	var reviewed *string
	err := r.w.QueryRow(ctx, `
		UPDATE entity_alias_proposals
		SET status = $3, reviewed_at = NOW()
		WHERE tenant_id = $1::uuid AND id = $2 AND status = 'pending'
		RETURNING id, kind, alias_key, source_entity_id::text, target_entity_id::text,
		          COALESCE(source_memory_id, 0), status, confidence, reason,
		          COALESCE(model, ''), metadata, created_at, reviewed_at
	`, tenantID, id, status).Scan(
		&row.ID, &row.Kind, &row.AliasKey, &row.SourceEntityID, &row.TargetEntityID,
		&row.SourceMemoryID, &row.Status, &row.Confidence, &row.Reason, &row.Model,
		&meta, &row.CreatedAt, &reviewed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("entity alias proposal not found or not pending")
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(meta, &row.Metadata)
	return &row, nil
}
