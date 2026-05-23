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
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "name", "role"}).
		AddRow(keyID, uuid.New().String(), "rot", "user")
	mock.ExpectQuery(`admin_rotate_api_key`).
		WithArgs(keyID, pgxmock.AnyArg(), "rot").
		WillReturnRows(rows)

	svc := NewAdminService(repository.NewAdminRepository(mock))
	resp, err := svc.RotateAPIKey(context.Background(), keyID, "rot")
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
