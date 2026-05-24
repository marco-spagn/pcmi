package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/marco-spagn/pcmi/internal/model"
)

// AdminQuerier is the DB surface required by AdminRepository (*pgxpool.Pool and pgxmock pools satisfy it).
type AdminQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// AdminAPIKeyOverview is a redacted row for operator CLI (no raw secrets).
type AdminAPIKeyOverview struct {
	TenantID   string
	TenantSlug string
	ID         string
	Name       string
	Role       string
	HashPrefix string
	IsActive   bool
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
}

type AdminRepository struct {
	db AdminQuerier
}

func NewAdminRepository(db AdminQuerier) *AdminRepository {
	return &AdminRepository{db: db}
}

func (r *AdminRepository) CreateTenant(ctx context.Context, slug, name string, settings map[string]interface{}) (*model.TenantResponse, error) {
	if settings == nil {
		settings = map[string]interface{}{}
	}
	var t model.TenantResponse
	err := r.db.QueryRow(ctx,
		`SELECT id::text, slug, name FROM admin_create_tenant($1, $2, $3)`,
		slug, name, settings,
	).Scan(&t.ID, &t.Slug, &t.Name)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	t.Settings = settings
	return &t, nil
}

const sortKeyTenantCreatedAt = model.SortKeyCreatedAtDesc

func (r *AdminRepository) ListTenants(ctx context.Context, page model.PageRequest) ([]model.TenantResponse, model.PageResponse, error) {
	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	q := `SELECT id::text, slug, name, settings, created_at FROM tenants WHERE 1=1`
	args := []any{}
	argN := 1
	clause, clauseArgs, err := KeysetTimeClause(page.Cursor, sortKeyTenantCreatedAt, "created_at", argN)
	if err != nil {
		return nil, model.PageResponse{}, err
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, argN)
	args = append(args, FetchLimit(limit))

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, model.PageResponse{}, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var out []model.TenantResponse
	for rows.Next() {
		var t model.TenantResponse
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Settings, &t.CreatedAt); err != nil {
			return nil, model.PageResponse{}, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, model.PageResponse{}, err
	}
	trimmed, pageResp, err := model.FinishInt64Page(out, limit, sortKeyTenantCreatedAt,
		func(t model.TenantResponse) int64 { return 0 },
		func(t model.TenantResponse) time.Time { return t.CreatedAt },
	)
	return trimmed, pageResp, err
}

func (r *AdminRepository) RotateAPIKey(ctx context.Context, keyID, keyHash, name string) (*model.APIKeyRotateResponse, error) {
	var resp model.APIKeyRotateResponse
	var previousID string
	var graceEnds *time.Time
	err := r.db.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, role, previous_key_id::text, grace_ends_at
		 FROM admin_rotate_api_key($1::uuid, $2, $3)`,
		keyID, keyHash, name,
	).Scan(&resp.ID, &resp.TenantID, &resp.Name, &resp.Role, &previousID, &graceEnds)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("api key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("rotate api key: %w", err)
	}
	resp.PreviousKeyID = previousID
	resp.RotationGraceEndsAt = graceEnds
	return &resp, nil
}

func (r *AdminRepository) RevokeAPIKey(ctx context.Context, keyID string) error {
	var id string
	err := r.db.QueryRow(ctx,
		`SELECT id::text FROM admin_revoke_api_key($1::uuid)`,
		keyID,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("api key not found")
	}
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}

func (r *AdminRepository) AuditAPIKeyRotation(ctx context.Context, tenantID, newKeyID, previousKeyID, path, method string, statusCode int, ip string) error {
	_, err := r.db.Exec(ctx,
		`SELECT admin_audit_api_key_rotation($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7)`,
		tenantID, newKeyID, previousKeyID, path, method, statusCode, ip,
	)
	if err != nil {
		return fmt.Errorf("audit api key rotation: %w", err)
	}
	return nil
}

func (r *AdminRepository) CreateAPIKey(ctx context.Context, tenantID, keyHash, name, role string, expiresAt *string) (*model.APIKeyRotateResponse, error) {
	var resp model.APIKeyRotateResponse
	err := r.db.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, role FROM admin_create_api_key($1::uuid, $2, $3, $4, $5::timestamptz)`,
		tenantID, keyHash, name, role, expiresAt,
	).Scan(&resp.ID, &resp.TenantID, &resp.Name, &resp.Role)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &resp, nil
}

func (r *AdminRepository) ListAPIKeys(ctx context.Context, tenantID string, page model.PageRequest) ([]map[string]interface{}, model.PageResponse, error) {
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	sortKey := sortKeyTenantCreatedAt

	q := `
		SELECT id::text, name, role, is_active, expires_at, created_at, last_used_at,
		       rotated_to::text, rotation_grace_ends_at, last_used_ip
		FROM api_keys
		WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	clause, clauseArgs, err := KeysetTimeClause(page.Cursor, sortKey, "created_at", argN)
	if err != nil {
		return nil, model.PageResponse{}, err
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, argN)
	args = append(args, FetchLimit(limit))

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, model.PageResponse{}, err
	}
	defer rows.Close()

	type keyRow struct {
		row map[string]interface{}
		at  time.Time
	}
	var scanned []keyRow
	for rows.Next() {
		var id, name, role string
		var active bool
		var expiresAt, createdAt, lastUsed, rotatedTo, graceEnds, lastIP interface{}
		var created time.Time
		if err := rows.Scan(&id, &name, &role, &active, &expiresAt, &createdAt, &lastUsed, &rotatedTo, &graceEnds, &lastIP); err != nil {
			return nil, model.PageResponse{}, err
		}
		if ts, ok := createdAt.(time.Time); ok {
			created = ts
		}
		scanned = append(scanned, keyRow{
			at: created,
			row: map[string]interface{}{
				"id": id, "name": name, "role": role, "is_active": active,
				"expires_at": expiresAt, "created_at": createdAt, "last_used_at": lastUsed,
				"rotated_to": rotatedTo, "rotation_grace_ends_at": graceEnds, "last_used_ip": lastIP,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, model.PageResponse{}, err
	}
	trimmed, pageResp, err := model.FinishInt64Page(scanned, limit, sortKey,
		func(k keyRow) int64 { return 0 },
		func(k keyRow) time.Time { return k.at },
	)
	if err != nil {
		return nil, model.PageResponse{}, err
	}
	out := make([]map[string]interface{}, len(trimmed))
	for i, k := range trimmed {
		out[i] = k.row
	}
	return out, pageResp, nil
}

func (r *AdminRepository) setTenantContext(ctx context.Context, tenantID string) error {
	_, err := r.db.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID)
	if err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}
	return nil
}

func (r *AdminRepository) listAPIKeysOverview(ctx context.Context, tenantID, tenantSlug string, limit int) ([]AdminAPIKeyOverview, error) {
	if err := r.setTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT k.id::text, k.name, k.role, k.is_active,
		       LEFT(k.key_hash, 8) AS hash_prefix,
		       k.created_at, k.expires_at, k.last_used_at
		FROM api_keys k
		WHERE k.tenant_id = $1::uuid
		ORDER BY k.created_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []AdminAPIKeyOverview
	for rows.Next() {
		var row AdminAPIKeyOverview
		row.TenantID = tenantID
		row.TenantSlug = tenantSlug
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Role, &row.IsActive, &row.HashPrefix,
			&row.CreatedAt, &row.ExpiresAt, &row.LastUsedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListAllAPIKeysOverview lists tenants (admin_list_tenants) and their API keys with hash prefix only.
// tenantFilter matches tenant slug or UUID; empty means all tenants up to tenantLimit.
func (r *AdminRepository) ListAllAPIKeysOverview(ctx context.Context, tenantLimit, keysPerTenant int, tenantFilter string) ([]AdminAPIKeyOverview, error) {
	tenants, _, err := r.ListTenants(ctx, model.PageRequest{Limit: tenantLimit})
	if err != nil {
		return nil, err
	}
	var out []AdminAPIKeyOverview
	for _, t := range tenants {
		if tenantFilter != "" && tenantFilter != t.Slug && tenantFilter != t.ID {
			continue
		}
		keys, err := r.listAPIKeysOverview(ctx, t.ID, t.Slug, keysPerTenant)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
	}
	return out, nil
}
