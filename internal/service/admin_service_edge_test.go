package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

func TestAdminService_RotateAPIKey_attachesRawKey(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	keyID := uuid.New().String()
	tenantID := uuid.New().String()
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "name", "role", "previous_key_id", "grace_ends_at"}).
		AddRow(keyID, tenantID, "rot", "user", keyID, nil)
	mock.ExpectQuery(`admin_rotate_api_key`).
		WithArgs(keyID, pgxmock.AnyArg(), "rot").
		WillReturnRows(rows)

	svc := NewAdminService(repository.NewAdminRepository(mock))
	resp, err := svc.RotateAPIKey(context.Background(), keyID, "rot", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.APIKey, "pcmi_") {
		t.Fatalf("api_key=%q", resp.APIKey)
	}
	if resp.ID != keyID {
		t.Fatalf("id=%q", resp.ID)
	}
}

func TestAdminService_RevokeAPIKey(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	keyID := uuid.New().String()
	mock.ExpectQuery(`admin_revoke_api_key`).
		WithArgs(keyID).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(keyID))

	svc := NewAdminService(repository.NewAdminRepository(mock))
	if err := svc.RevokeAPIKey(context.Background(), keyID); err != nil {
		t.Fatal(err)
	}
}

func TestAdminService_RotateAPIKey_auditsWhenRequested(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	keyID := uuid.New().String()
	tenantID := uuid.New().String()
	prev := uuid.New().String()
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "name", "role", "previous_key_id", "grace_ends_at"}).
		AddRow(keyID, tenantID, "rot", "user", prev, nil)
	mock.ExpectQuery(`admin_rotate_api_key`).
		WithArgs(keyID, pgxmock.AnyArg(), "rot").
		WillReturnRows(rows)
	mock.ExpectExec(`admin_audit_api_key_rotation`).
		WithArgs(tenantID, keyID, prev, "/keys", "POST", 200, "10.0.0.1").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))

	svc := NewAdminService(repository.NewAdminRepository(mock))
	if _, err := svc.RotateAPIKey(context.Background(), keyID, "rot", "/keys", "POST", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
}

func TestAdminService_CreateAPIKey_attachesRawKey(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	newID := uuid.New().String()
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "name", "role"}).
		AddRow(newID, tenantID, "new", "admin")
	mock.ExpectQuery(`admin_create_api_key`).
		WithArgs(tenantID, pgxmock.AnyArg(), "new", "admin", pgxmock.AnyArg()).
		WillReturnRows(rows)

	svc := NewAdminService(repository.NewAdminRepository(mock))
	resp, err := svc.CreateAPIKey(context.Background(), &model.APIKeyCreateRequest{
		TenantID: tenantID,
		Name:     "new",
		Role:     "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.APIKey, "pcmi_") || resp.ID != newID {
		t.Fatalf("unexpected %+v", resp)
	}
}
