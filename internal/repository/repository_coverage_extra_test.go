package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestNewMemoryRepositoryFromDB(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })
	r := NewMemoryRepositoryFromDB(mock, mock)
	if r == nil || r.r == nil || r.w == nil {
		t.Fatal("NewMemoryRepositoryFromDB returned nil or nil fields")
	}
}

func TestNewMemoryRepositoryFromDB_ReadNil(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })
	r := NewMemoryRepositoryFromDB(mock, nil)
	if r == nil {
		t.Fatal("NewMemoryRepositoryFromDB returned nil")
	}
}
func TestMemoryRepository_GetTenantDedupMode_NoRows(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectQuery(`SELECT settings FROM tenants`).
		WithArgs(tenant).
		WillReturnError(pgx.ErrNoRows)

	r := NewMemoryRepositoryFromDB(mock, mock)
	mode, err := r.GetTenantDedupMode(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if mode != model.DedupModeNone {
		t.Fatalf("got %q, want none", mode)
	}
}

func TestMemoryRepository_GetTenantDedupMode_WithMode(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectQuery(`SELECT settings FROM tenants`).
		WithArgs(tenant).
		WillReturnRows(pgxmock.NewRows([]string{"settings"}).AddRow([]byte(`{"dedup_mode":"skip"}`)))

	r := NewMemoryRepositoryFromDB(mock, mock)
	mode, err := r.GetTenantDedupMode(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if mode != model.DedupModeSkip {
		t.Fatalf("got %q, want skip", mode)
	}
}

func TestMemoryRepository_GetTenantDedupMode_EmptySettings(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectQuery(`SELECT settings FROM tenants`).
		WithArgs(tenant).
		WillReturnRows(pgxmock.NewRows([]string{"settings"}).AddRow([]byte(`{}`)))

	r := NewMemoryRepositoryFromDB(mock, mock)
	mode, err := r.GetTenantDedupMode(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if mode != model.DedupModeNone {
		t.Fatalf("got %q, want none", mode)
	}
}

func TestMemoryRepository_FindCurrentByContentHash_Found(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	now := time.Now().UTC()
	row := pgxmock.NewRows([]string{
		"id", "tenant_id", "path", "content", "metadata", "tags",
		"embedding", "embedding_model", "embedding_space", "version", "valid_from", "valid_to",
		"source_agent_id", "source_event_id", "created_at", "content_encrypted",
		"importance", "access_count", "last_accessed_at",
	}).AddRow(int64(1), tenant, "root.x", "content", []byte(`{}`), []string{},
		nil, "unspecified", "default", 1, now, nil, nil, nil, now, false, 0.5, 0, nil)

	mock.ExpectQuery(`SELECT.*FROM memory_entries`).
		WithArgs(tenant, "abc123hash").
		WillReturnRows(row)

	r := NewMemoryRepositoryFromDB(mock, mock)
	entry, err := r.FindCurrentByContentHash(context.Background(), tenant, "abc123hash")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.ID != 1 {
		t.Fatalf("id=%d", entry.ID)
	}
}

func TestMemoryRepository_FindCurrentByContentHash_DBError(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectQuery(`SELECT.*FROM memory_entries`).
		WithArgs(tenant, "hash123").
		WillReturnError(errors.New("db error"))

	r := NewMemoryRepositoryFromDB(mock, mock)
	_, err := r.FindCurrentByContentHash(context.Background(), tenant, "hash123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMemoryRepository_UpsertDedupLink_Success(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectExec(`INSERT INTO memory_links`).
		WithArgs(tenant, "root.a", "root.b", model.DedupLinkType(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	r := NewMemoryRepositoryFromDB(mock, mock)
	err := r.UpsertDedupLink(context.Background(), tenant, "root.a", "root.b")
	if err != nil {
		t.Fatal(err)
	}
}

func TestMemoryRepository_UpsertDedupLink_DBError(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	mock.ExpectExec(`INSERT INTO memory_links`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("db error"))

	r := NewMemoryRepositoryFromDB(mock, mock)
	err := r.UpsertDedupLink(context.Background(), uuid.New().String(), "root.a", "root.b")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMemoryRepository_CountPathHistory_EmptyPath(t *testing.T) {
	t.Parallel()
	mock, _ := pgxmock.NewPool()
	t.Cleanup(func() { mock.Close() })

	r := NewMemoryRepositoryFromDB(mock, mock)
	_, err := r.CountPathHistory(context.Background(), uuid.New().String(), "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}
