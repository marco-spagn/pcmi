package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/model"
)

type AdminRepository struct {
	db *pgxpool.Pool
}

func NewAdminRepository(db *pgxpool.Pool) *AdminRepository {
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

func (r *AdminRepository) ListTenants(ctx context.Context, limit int) ([]model.TenantResponse, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id::text, slug, name, settings, created_at FROM admin_list_tenants($1)`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var out []model.TenantResponse
	for rows.Next() {
		var t model.TenantResponse
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Settings, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *AdminRepository) RotateAPIKey(ctx context.Context, keyID, keyHash, name string) (*model.APIKeyRotateResponse, error) {
	var resp model.APIKeyRotateResponse
	err := r.db.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, role FROM admin_rotate_api_key($1::uuid, $2, $3)`,
		keyID, keyHash, name,
	).Scan(&resp.ID, &resp.TenantID, &resp.Name, &resp.Role)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("api key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("rotate api key: %w", err)
	}
	return &resp, nil
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

func (r *AdminRepository) ListAPIKeys(ctx context.Context, tenantID string, limit int) ([]map[string]interface{}, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, role, is_active, expires_at, created_at, last_used_at
		FROM api_keys
		WHERE tenant_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var id, name, role string
		var active bool
		var expiresAt, createdAt, lastUsed interface{}
		if err := rows.Scan(&id, &name, &role, &active, &expiresAt, &createdAt, &lastUsed); err != nil {
			return nil, err
		}
		out = append(out, map[string]interface{}{
			"id": id, "name": name, "role": role, "is_active": active,
			"expires_at": expiresAt, "created_at": createdAt, "last_used_at": lastUsed,
		})
	}
	return out, rows.Err()
}
