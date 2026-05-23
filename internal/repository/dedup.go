package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/marco-spagn/pcmi/internal/model"
)

// GetTenantDedupMode reads tenants.settings.dedup_mode when present.
func (r *MemoryRepository) GetTenantDedupMode(ctx context.Context, tenantID string) (model.DedupMode, error) {
	var raw []byte
	err := r.r.QueryRow(ctx, `SELECT settings FROM tenants WHERE id = $1::uuid`, tenantID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.DedupModeNone, nil
		}
		return "", fmt.Errorf("tenant dedup mode: %w", err)
	}
	var settings map[string]interface{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &settings)
	}
	if settings == nil {
		return model.DedupModeNone, nil
	}
	if v, ok := settings["dedup_mode"].(string); ok && strings.TrimSpace(v) != "" {
		return model.ParseDedupMode(v)
	}
	return model.DedupModeNone, nil
}

// FindCurrentByContentHash returns a current memory row with the given content hash.
func (r *MemoryRepository) FindCurrentByContentHash(ctx context.Context, tenantID, hash string) (*model.MemoryEntry, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, nil
	}
	row := r.r.QueryRow(ctx, `
		SELECT `+memoryEntrySelectCols+`
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND content_hash = $2 AND valid_to IS NULL
		ORDER BY created_at DESC
		LIMIT 1`, tenantID, hash)
	e, err := r.scanMemoryEntry(row, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by content hash: %w", err)
	}
	return &e, nil
}

// MergeCurrentMetadata updates metadata/tags on the current version at path without bumping version.
func (r *MemoryRepository) MergeCurrentMetadata(ctx context.Context, tenantID, path string, metadata map[string]interface{}, tags []string) (*model.MemoryEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if tags == nil {
		tags = []string{}
	}
	tag, err := r.w.Exec(ctx, `
		UPDATE memory_entries
		SET metadata = metadata || $3::jsonb,
		    tags = (SELECT ARRAY(SELECT DISTINCT unnest(tags || $4::text[])))
		WHERE tenant_id = $1::uuid AND path = $2::ltree AND valid_to IS NULL`,
		tenantID, path, metadata, tags)
	if err != nil {
		return nil, fmt.Errorf("merge metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("memory not found at path %s", path)
	}
	return r.GetByPath(ctx, tenantID, path, nil, nil)
}

// UpsertDedupLink links fromPath to the canonical path for duplicate content.
func (r *MemoryRepository) UpsertDedupLink(ctx context.Context, tenantID, fromPath, toPath string) error {
	fromPath = strings.TrimSpace(fromPath)
	toPath = strings.TrimSpace(toPath)
	if fromPath == "" || toPath == "" {
		return fmt.Errorf("from_path and to_path are required")
	}
	meta := map[string]interface{}{"source": "dedup"}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = r.w.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, metadata)
		VALUES ($1::uuid, $2::ltree, $3::ltree, $4, $5)
		ON CONFLICT (tenant_id, from_path, to_path, link_type)
		DO UPDATE SET metadata = EXCLUDED.metadata`,
		tenantID, fromPath, toPath, model.DedupLinkType(), metaJSON)
	if err != nil {
		return fmt.Errorf("upsert dedup link: %w", err)
	}
	return nil
}
