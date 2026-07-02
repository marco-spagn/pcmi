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

	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
)

// ExtractionProfileRow is a tenant extraction profile stored in Postgres.
type ExtractionProfileRow struct {
	ProfileID   string          `json:"profile_id"`
	Version     int             `json:"version"`
	PathPrefix  string          `json:"path_prefix"`
	Profile     json.RawMessage `json:"profile"`
	Enabled     bool            `json:"enabled"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ExtractionRepository manages extraction_profiles and memory reads for extraction.
type ExtractionRepository struct {
	w *pgxpool.Pool
	r *pgxpool.Pool
}

func NewExtractionRepository(dbWrite, readReplica *pgxpool.Pool) *ExtractionRepository {
	r := readReplica
	if r == nil {
		r = dbWrite
	}
	return &ExtractionRepository{w: dbWrite, r: r}
}

func (r *ExtractionRepository) setTenant(ctx context.Context, tenantID string) error {
	_, err := r.w.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID)
	return err
}

// ListProfiles returns enabled and disabled profiles for a tenant.
func (r *ExtractionRepository) ListProfiles(ctx context.Context, tenantID string) ([]ExtractionProfileRow, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := r.r.Query(ctx, `
		SELECT profile_id, version, path_prefix::text, profile, enabled, updated_at
		FROM extraction_profiles
		WHERE tenant_id = $1::uuid
		ORDER BY profile_id ASC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list extraction profiles: %w", err)
	}
	defer rows.Close()
	var out []ExtractionProfileRow
	for rows.Next() {
		var row ExtractionProfileRow
		if err := rows.Scan(&row.ProfileID, &row.Version, &row.PathPrefix, &row.Profile, &row.Enabled, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan extraction profile: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// MatchProfile picks the most specific enabled profile for a memory path.
func (r *ExtractionRepository) MatchProfile(ctx context.Context, tenantID, path string) (*extraction.Profile, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", nil
	}
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, "", err
	}
	var raw []byte
	var pathPrefix string
	err := r.r.QueryRow(ctx, `
		SELECT profile, path_prefix::text
		FROM extraction_profiles
		WHERE tenant_id = $1::uuid
		  AND enabled = true
		  AND $2::ltree <@ path_prefix
		ORDER BY nlevel(path_prefix) DESC
		LIMIT 1`, tenantID, path).Scan(&raw, &pathPrefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("match extraction profile: %w", err)
	}
	p, err := extraction.ParseProfile(raw)
	if err != nil {
		return nil, "", err
	}
	return p, pathPrefix, nil
}

// UpsertProfile inserts or updates a tenant profile.
func (r *ExtractionRepository) UpsertProfile(ctx context.Context, tenantID, profileID, pathPrefix string, profile *extraction.Profile, enabled bool) (*ExtractionProfileRow, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, fmt.Errorf("profile_id is required")
	}
	pathPrefix = strings.TrimSpace(pathPrefix)
	if pathPrefix == "" {
		pathPrefix = "root"
	}
	if err := model.ValidateLtreePath(pathPrefix); err != nil {
		return nil, fmt.Errorf("invalid path_prefix: %w", err)
	}
	if profile == nil {
		return nil, fmt.Errorf("profile is required")
	}
	profile.ProfileID = profileID
	validated, err := extraction.ValidateProfile(profile)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(validated)
	if err != nil {
		return nil, err
	}
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var row ExtractionProfileRow
	err = r.w.QueryRow(ctx, `
		INSERT INTO extraction_profiles (tenant_id, profile_id, version, path_prefix, profile, enabled, updated_at)
		VALUES ($1::uuid, $2, $3, $4::ltree, $5::jsonb, $6, NOW())
		ON CONFLICT (tenant_id, profile_id) DO UPDATE SET
			version = EXCLUDED.version,
			path_prefix = EXCLUDED.path_prefix,
			profile = EXCLUDED.profile,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
		RETURNING profile_id, version, path_prefix::text, profile, enabled, updated_at`,
		tenantID, profileID, validated.Version, pathPrefix, raw, enabled,
	).Scan(&row.ProfileID, &row.Version, &row.PathPrefix, &row.Profile, &row.Enabled, &row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert extraction profile: %w", err)
	}
	return &row, nil
}

// DeleteProfile removes a profile by id for the tenant.
func (r *ExtractionRepository) DeleteProfile(ctx context.Context, tenantID, profileID string) (bool, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return false, err
	}
	tag, err := r.w.Exec(ctx, `
		DELETE FROM extraction_profiles
		WHERE tenant_id = $1::uuid AND profile_id = $2`, tenantID, profileID)
	if err != nil {
		return false, fmt.Errorf("delete extraction profile: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GetCurrentMemoryByID loads the current memory row for extraction.
func (r *ExtractionRepository) GetCurrentMemoryByID(ctx context.Context, tenantID string, memoryID int64) (*model.MemoryEntry, error) {
	if err := r.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	row := r.r.QueryRow(ctx, `
		SELECT `+memoryEntrySelectCols+`
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND id = $2 AND valid_to IS NULL`, tenantID, memoryID)
	memRepo := &MemoryRepository{r: r.r, w: r.w}
	e, err := memRepo.scanMemoryEntry(row, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get memory by id: %w", err)
	}
	return &e, nil
}
