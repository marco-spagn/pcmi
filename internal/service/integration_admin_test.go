//go:build integration

package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

func adminTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — skipping integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func adminSetTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}
}

func TestIntegration_AdminService(t *testing.T) {
	ctx := context.Background()
	pool := adminTestPool(t)
	adminRepo := repository.NewAdminRepository(pool)
	svc := NewAdminService(adminRepo)

	if _, _, err := svc.ListTenants(ctx, model.PageRequest{Limit: 5}); err != nil {
		t.Fatalf("list tenants: %v", err)
	}

	slug := fmt.Sprintf("adm-%d", time.Now().UnixNano())
	tenant, err := svc.CreateTenant(ctx, &model.TenantCreateRequest{Slug: slug, Name: "CI Tenant"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	adminSetTenant(t, ctx, pool, tenant.ID)
	_, err = svc.CreateAPIKey(ctx, &model.APIKeyCreateRequest{
		TenantID: tenant.ID, Name: "ci-key", Role: "user",
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	keys, _, err := svc.ListAPIKeys(ctx, tenant.ID, model.PageRequest{Limit: 10})
	if err != nil || len(keys) < 1 {
		t.Fatalf("list keys: err=%v n=%d", err, len(keys))
	}

	rawID := keys[0]["id"]
	keyID, _ := rawID.(string)
	if keyID == "" {
		keyID = fmt.Sprintf("%v", rawID)
	}
	if keyID == "" {
		t.Fatalf("missing key id: %#v", keys[0])
	}
	if _, err := svc.RotateAPIKey(ctx, keyID, "rotated", "", "", ""); err != nil {
		t.Fatalf("rotate: %v", err)
	}
}
