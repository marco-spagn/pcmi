package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestAdminRepository_RotateAPIKey_notFound(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	keyID := uuid.New().String()
	mock.ExpectQuery(`admin_rotate_api_key`).
		WithArgs(keyID, "hash", "name").
		WillReturnError(pgx.ErrNoRows)

	repo := &AdminRepository{db: mock}
	_, err = repo.RotateAPIKey(context.Background(), keyID, "hash", "name")
	if err == nil || err.Error() != "api key not found" {
		t.Fatalf("got err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRepository_RotateAPIKey_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	keyID := uuid.New().String()
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "name", "role", "previous_key_id", "grace_ends_at"}).
		AddRow(keyID, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "rotated", "admin", "", nil)
	mock.ExpectQuery(`admin_rotate_api_key`).
		WithArgs(keyID, "newhash", "rotated").
		WillReturnRows(rows)

	repo := &AdminRepository{db: mock}
	got, err := repo.RotateAPIKey(context.Background(), keyID, "newhash", "rotated")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != keyID || got.Role != "admin" || got.Name != "rotated" {
		t.Fatalf("unexpected %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRepository_CreateAPIKey_dbError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`admin_create_api_key`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("constraint violation"))

	repo := &AdminRepository{db: mock}
	_, err = repo.CreateAPIKey(context.Background(), uuid.New().String(), "h", "n", "user", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminRepository_ListAPIKeys_empty(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	mock.ExpectQuery(`FROM api_keys`).
		WithArgs(tenantID, 10).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "name", "role", "is_active", "expires_at", "created_at", "last_used_at",
			"rotated_to", "rotation_grace_ends_at", "last_used_ip",
		}))

	repo := &AdminRepository{db: mock}
	got, err := repo.ListAPIKeys(context.Background(), tenantID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}

func TestAdminRepository_ListAPIKeys_row(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	created := time.Unix(1700000000, 0).UTC()
	rows := pgxmock.NewRows([]string{
		"id", "name", "role", "is_active", "expires_at", "created_at", "last_used_at",
		"rotated_to", "rotation_grace_ends_at", "last_used_ip",
	}).AddRow("kid", "ci", "user", true, nil, created, nil, nil, nil, nil)
	mock.ExpectQuery(`FROM api_keys`).
		WithArgs(tenantID, 5).
		WillReturnRows(rows)

	repo := &AdminRepository{db: mock}
	got, err := repo.ListAPIKeys(context.Background(), tenantID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["name"] != "ci" || got[0]["is_active"] != true {
		t.Fatalf("unexpected %+v", got[0])
	}
}

func TestAdminRepository_CreateTenant_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	rows := pgxmock.NewRows([]string{"id", "slug", "name"}).
		AddRow("tid", "acme", "Acme")
	mock.ExpectQuery(`admin_create_tenant`).
		WithArgs("acme", "Acme", map[string]interface{}{}).
		WillReturnRows(rows)

	repo := &AdminRepository{db: mock}
	got, err := repo.CreateTenant(context.Background(), "acme", "Acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "tid" || got.Slug != "acme" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestAdminRepository_ListAllAPIKeysOverview_tenantFilter(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	tenantB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	tenantRows := pgxmock.NewRows([]string{"id", "slug", "name", "settings", "created_at"}).
		AddRow(tenantA, "alpha", "Alpha", map[string]interface{}{}, time.Now()).
		AddRow(tenantB, "beta", "Beta", map[string]interface{}{}, time.Now())
	mock.ExpectQuery(`admin_list_tenants`).WithArgs(10).WillReturnRows(tenantRows)

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantA).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	keyRows := pgxmock.NewRows([]string{
		"id", "name", "role", "is_active", "hash_prefix", "created_at", "expires_at", "last_used_at",
	}).AddRow("k1", "key", "admin", true, "abcd1234", time.Now(), nil, nil)
	mock.ExpectQuery(`FROM api_keys k`).WithArgs(tenantA, 5).WillReturnRows(keyRows)

	repo := &AdminRepository{db: mock}
	got, err := repo.ListAllAPIKeysOverview(context.Background(), 10, 5, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TenantSlug != "alpha" || got[0].HashPrefix != "abcd1234" {
		t.Fatalf("unexpected %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
