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

// EntityRegistryRepository manages canonical entities, aliases, and snapshots.
type EntityRegistryRepository struct {
	w *pgxpool.Pool
	r *pgxpool.Pool
}

func NewEntityRegistryRepository(dbWrite, readReplica *pgxpool.Pool) *EntityRegistryRepository {
	r := readReplica
	if r == nil {
		r = dbWrite
	}
	return &EntityRegistryRepository{w: dbWrite, r: r}
}

func (repo *EntityRegistryRepository) setTenant(ctx context.Context, tenantID string) error {
	_, err := repo.w.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID)
	return err
}

// ResolveCanonicalKey returns the canonical key for kind+alias, or empty if unknown.
func (repo *EntityRegistryRepository) ResolveCanonicalKey(ctx context.Context, tenantID, kind, aliasKey string) (string, error) {
	kind = strings.TrimSpace(kind)
	aliasKey = strings.TrimSpace(aliasKey)
	if kind == "" || aliasKey == "" {
		return "", nil
	}
	var canonical string
	err := repo.r.QueryRow(ctx, `
		SELECT er.canonical_key
		FROM entity_aliases ea
		JOIN entity_registry er ON er.id = ea.entity_id AND er.tenant_id = ea.tenant_id
		WHERE ea.tenant_id = $1::uuid
		  AND ea.kind = $2
		  AND ea.alias_key = $3
		  AND ea.valid_to IS NULL
	`, tenantID, kind, aliasKey).Scan(&canonical)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return canonical, nil
}

// GetByID returns a registry row by UUID.
func (repo *EntityRegistryRepository) GetByID(ctx context.Context, tenantID, id string) (*model.EntityRegistry, error) {
	var row model.EntityRegistry
	var meta []byte
	err := repo.r.QueryRow(ctx, `
		SELECT id::text, kind, canonical_key, COALESCE(display_name, ''), metadata, created_at, updated_at
		FROM entity_registry
		WHERE tenant_id = $1::uuid AND id = $2::uuid
	`, tenantID, id).Scan(
		&row.ID, &row.Kind, &row.CanonicalKey, &row.DisplayName, &meta, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("entity not found")
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(meta, &row.Metadata)
	return &row, nil
}

// GetByCanonical returns a registry row by kind + canonical key.
func (repo *EntityRegistryRepository) GetByCanonical(ctx context.Context, tenantID, kind, canonicalKey string) (*model.EntityRegistry, error) {
	var row model.EntityRegistry
	var meta []byte
	err := repo.r.QueryRow(ctx, `
		SELECT id::text, kind, canonical_key, COALESCE(display_name, ''), metadata, created_at, updated_at
		FROM entity_registry
		WHERE tenant_id = $1::uuid AND kind = $2 AND canonical_key = $3
	`, tenantID, kind, canonicalKey).Scan(
		&row.ID, &row.Kind, &row.CanonicalKey, &row.DisplayName, &meta, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(meta, &row.Metadata)
	return &row, nil
}

// UpsertCanonical creates or returns an entity registry row.
func (repo *EntityRegistryRepository) UpsertCanonical(ctx context.Context, tenantID, kind, canonicalKey, displayName string, metadata map[string]interface{}) (*model.EntityRegistry, error) {
	if err := repo.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	var row model.EntityRegistry
	var metaOut []byte
	err = repo.w.QueryRow(ctx, `
		INSERT INTO entity_registry (tenant_id, kind, canonical_key, display_name, metadata)
		VALUES ($1::uuid, $2, $3, NULLIF($4, ''), $5::jsonb)
		ON CONFLICT (tenant_id, kind, canonical_key) DO UPDATE SET
			display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), entity_registry.display_name),
			metadata = entity_registry.metadata || EXCLUDED.metadata,
			updated_at = NOW()
		RETURNING id::text, kind, canonical_key, COALESCE(display_name, ''), metadata, created_at, updated_at
	`, tenantID, kind, canonicalKey, displayName, metaJSON).Scan(
		&row.ID, &row.Kind, &row.CanonicalKey, &row.DisplayName, &metaOut, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(metaOut, &row.Metadata)
	return &row, nil
}

// UpsertActiveAlias adds or refreshes an active alias for an entity.
func (repo *EntityRegistryRepository) UpsertActiveAlias(ctx context.Context, tenantID, entityID, kind, aliasKey, source string, confidence float64, metadata map[string]interface{}) error {
	if err := repo.setTenant(ctx, tenantID); err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = repo.w.Exec(ctx, `
		INSERT INTO entity_aliases (tenant_id, entity_id, kind, alias_key, source, confidence, metadata)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::jsonb)
		ON CONFLICT (tenant_id, kind, alias_key) WHERE (valid_to IS NULL) DO UPDATE SET
			entity_id = EXCLUDED.entity_id,
			source = EXCLUDED.source,
			confidence = GREATEST(entity_aliases.confidence, EXCLUDED.confidence),
			metadata = entity_aliases.metadata || EXCLUDED.metadata
	`, tenantID, entityID, kind, aliasKey, source, confidence, metaJSON)
	return err
}

// InsertSnapshot records entity evolution from an extraction event.
func (repo *EntityRegistryRepository) InsertSnapshot(ctx context.Context, tenantID, entityID string, memoryID int64, memoryVersion int, profileID, slot, rawKey string, attributes map[string]interface{}, confidence float64) error {
	if err := repo.setTenant(ctx, tenantID); err != nil {
		return err
	}
	if attributes == nil {
		attributes = map[string]interface{}{}
	}
	attrJSON, err := json.Marshal(attributes)
	if err != nil {
		return err
	}
	_, err = repo.w.Exec(ctx, `
		INSERT INTO entity_snapshots (
			tenant_id, entity_id, memory_id, memory_version, profile_id, slot, raw_key, attributes, confidence
		) VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8::jsonb, $9)
		ON CONFLICT (tenant_id, entity_id, memory_id, memory_version, slot) DO UPDATE SET
			raw_key = EXCLUDED.raw_key,
			attributes = EXCLUDED.attributes,
			confidence = EXCLUDED.confidence,
			profile_id = EXCLUDED.profile_id
	`, tenantID, entityID, memoryID, memoryVersion, profileID, slot, rawKey, attrJSON, confidence)
	return err
}

// ListEntities returns canonical entities for a tenant, optionally filtered by kind.
func (repo *EntityRegistryRepository) ListEntities(ctx context.Context, tenantID, kind string, limit int) ([]model.EntityRegistry, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT id::text, kind, canonical_key, COALESCE(display_name, ''), metadata, created_at, updated_at
		FROM entity_registry
		WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	if strings.TrimSpace(kind) != "" {
		q += ` AND kind = $2`
		args = append(args, kind)
	}
	q += fmt.Sprintf(` ORDER BY updated_at DESC LIMIT %d`, limit)

	rows, err := repo.r.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EntityRegistry
	for rows.Next() {
		var row model.EntityRegistry
		var meta []byte
		if err := rows.Scan(&row.ID, &row.Kind, &row.CanonicalKey, &row.DisplayName, &meta, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meta, &row.Metadata)
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListSnapshots returns evolution history for an entity.
func (repo *EntityRegistryRepository) ListSnapshots(ctx context.Context, tenantID, entityID string, limit int) ([]model.EntitySnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := repo.r.Query(ctx, `
		SELECT id, entity_id::text, memory_id, memory_version, COALESCE(profile_id, ''), slot,
		       COALESCE(raw_key, ''), attributes, confidence, created_at
		FROM entity_snapshots
		WHERE tenant_id = $1::uuid AND entity_id = $2::uuid
		ORDER BY created_at DESC, memory_version DESC
		LIMIT $3
	`, tenantID, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EntitySnapshot
	for rows.Next() {
		var row model.EntitySnapshot
		var attrs []byte
		if err := rows.Scan(&row.ID, &row.EntityID, &row.MemoryID, &row.MemoryVersion, &row.ProfileID, &row.Slot, &row.RawKey, &attrs, &row.Confidence, &row.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(attrs, &row.Attributes)
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListActiveAliases returns active aliases for an entity.
func (repo *EntityRegistryRepository) ListActiveAliases(ctx context.Context, tenantID, entityID string) ([]model.EntityAlias, error) {
	rows, err := repo.r.Query(ctx, `
		SELECT id::text, entity_id::text, kind, alias_key, source, confidence, valid_from, valid_to, metadata
		FROM entity_aliases
		WHERE tenant_id = $1::uuid AND entity_id = $2::uuid AND valid_to IS NULL
		ORDER BY alias_key
	`, tenantID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntityAliases(rows)
}

// ExpandEntityKeys returns canonical key plus all active alias keys for graph lookup.
func (repo *EntityRegistryRepository) ExpandEntityKeys(ctx context.Context, tenantID, kind, key string) ([]string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	canonical, err := repo.ResolveCanonicalKey(ctx, tenantID, kind, key)
	if err != nil {
		return nil, err
	}
	root := key
	if canonical != "" {
		root = canonical
	}
	rows, err := repo.r.Query(ctx, `
		SELECT alias_key FROM entity_aliases
		WHERE tenant_id = $1::uuid AND kind = $2 AND valid_to IS NULL
		  AND entity_id = (
		    SELECT id FROM entity_registry
		    WHERE tenant_id = $1::uuid AND kind = $2 AND canonical_key = $3
		    LIMIT 1
		  )
		UNION SELECT $3
	`, tenantID, kind, root)
	if err != nil {
		return []string{root}, nil
	}
	defer rows.Close()
	keys := map[string]struct{}{root: {}}
	for rows.Next() {
		var ak string
		if err := rows.Scan(&ak); err != nil {
			continue
		}
		keys[ak] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	return out, nil
}

// MergeEntityAliasInGraph calls the AGE helper to rewire mentions and add same_as.
func (repo *EntityRegistryRepository) MergeEntityAliasInGraph(ctx context.Context, tenantID, kind, aliasKey, canonicalKey string) error {
	if err := repo.setTenant(ctx, tenantID); err != nil {
		return err
	}
	_, err := repo.w.Exec(ctx, `SELECT public.merge_entity_alias_in_graph($1::uuid, $2, $3, $4)`,
		tenantID, kind, aliasKey, canonicalKey)
	return err
}

func scanEntityAliases(rows pgx.Rows) ([]model.EntityAlias, error) {
	var out []model.EntityAlias
	for rows.Next() {
		var row model.EntityAlias
		var meta []byte
		if err := rows.Scan(&row.ID, &row.EntityID, &row.Kind, &row.AliasKey, &row.Source, &row.Confidence, &row.ValidFrom, &row.ValidTo, &meta); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meta, &row.Metadata)
		out = append(out, row)
	}
	return out, rows.Err()
}
