package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestLinksRepository_NewLinksRepositoryReadOnly(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	repo := NewLinksRepositoryReadOnly(mock)
	if repo == nil {
		t.Fatal("NewLinksRepositoryReadOnly returned nil")
	}
}

func TestLinksRepository_Count(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_links`).
		WithArgs(tenant).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(7))

	repo := NewLinksRepositoryReadOnly(mock)
	n, err := repo.Count(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("got %d, want 7", n)
	}
}

func TestLinksRepository_CountError(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_links`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("db down"))

	repo := NewLinksRepositoryReadOnly(mock)
	if _, err := repo.Count(context.Background(), uuid.New().String()); err == nil {
		t.Fatal("expected error")
	}
}

func TestStatsRepository_NewFromRowDB(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	repo := NewStatsRepositoryFromRowDB(mock)
	if repo == nil {
		t.Fatal("NewStatsRepositoryFromRowDB returned nil")
	}
}

func TestIdempotencyRepository_Put_New(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectExec(`INSERT INTO idempotency_cache`).
		WithArgs("my-key", tenant, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := NewIdempotencyRepository(mock)
	if err := repo.Put(context.Background(), tenant, "my-key", json.RawMessage(`{"result":"ok"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryRepository_FindCurrentByContentHash_Empty(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	repo := NewMemoryRepositoryFromDB(mock, mock)
	entry, err := repo.FindCurrentByContentHash(context.Background(), uuid.New().String(), "")
	if err != nil {
		t.Fatal(err)
	}
	if entry != nil {
		t.Fatal("expected nil for empty hash")
	}
}

func TestUpsertDedupLink_EmptyPaths(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	repo := NewMemoryRepositoryFromDB(mock, mock)
	err = repo.UpsertDedupLink(context.Background(), uuid.New().String(), "", "to")
	if err == nil {
		t.Fatal("expected error for empty from_path")
	}
	err = repo.UpsertDedupLink(context.Background(), uuid.New().String(), "from", "")
	if err == nil {
		t.Fatal("expected error for empty to_path")
	}
}

func TestPagination_Functions(t *testing.T) {
	t.Parallel()

	t.Run("KeysetIDClause", func(t *testing.T) {
		t.Parallel()
		cur := model.Cursor{LastID: 42, SortKey: model.SortKeyIDDesc}
		clause, args, err := KeysetIDClause(cur, model.SortKeyIDDesc, 2)
		if err != nil {
			t.Fatal(err)
		}
		if clause == "" {
			t.Fatal("expected non-empty clause")
		}
		if len(args) != 1 || args[0] != int64(42) {
			t.Fatalf("unexpected args %v", args)
		}
	})

	t.Run("KeysetIDClause_zeroCursor", func(t *testing.T) {
		t.Parallel()
		clause, args, err := KeysetIDClause(model.Cursor{}, model.SortKeyIDDesc, 1)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "" {
			t.Fatalf("expected empty clause, got %q", clause)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args, got %v", args)
		}
	})

	t.Run("KeysetTimeClause", func(t *testing.T) {
		t.Parallel()
		clause, args, err := KeysetTimeClause(model.Cursor{}, model.SortKeyCreatedAtDesc, "created_at", 1)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "" {
			t.Fatalf("expected empty clause for zero cursor, got %q", clause)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args, got %v", args)
		}
	})

	t.Run("FetchLimit", func(t *testing.T) {
		t.Parallel()
		if got := FetchLimit(5); got != 6 {
			t.Fatalf("FetchLimit(5)=%d, want 6", got)
		}
		if got := FetchLimit(0); got != 2 {
			t.Fatalf("FetchLimit(0)=%d, want 2", got)
		}
	})
}
