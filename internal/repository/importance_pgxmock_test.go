package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

func TestMemoryRepository_UpdateImportance_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	mock.ExpectExec(`UPDATE memory_entries`).
		WithArgs(tenantID, "root.note", 0.9).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	repo := &MemoryRepository{w: mock, r: mock}
	if err := repo.UpdateImportance(context.Background(), tenantID, "root.note", 0.9); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryRepository_UpdateImportance_notFound(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	mock.ExpectExec(`UPDATE memory_entries`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	repo := &MemoryRepository{w: mock, r: mock}
	err = repo.UpdateImportance(context.Background(), uuid.New().String(), "root.missing", 0.5)
	if err == nil || err.Error() != "memory not found for path root.missing" {
		t.Fatalf("got err=%v", err)
	}
}

func TestMemoryRepository_UpdateImportance_emptyPath(t *testing.T) {
	t.Parallel()
	repo := &MemoryRepository{w: nil, r: nil}
	if err := repo.UpdateImportance(context.Background(), uuid.New().String(), "  ", 0.5); err == nil {
		t.Fatal("expected path required")
	}
}

func TestMemoryRepository_touchRetrievedMemories_empty(t *testing.T) {
	t.Parallel()
	repo := &MemoryRepository{w: nil, r: nil}
	if err := repo.touchRetrievedMemories(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryRepository_touchRetrievedMemories_exec(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	mock.ExpectExec(`access_count = access_count \+ 1`).
		WithArgs([]int64{1, 2}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	repo := &MemoryRepository{w: mock, r: mock}
	if err := repo.touchRetrievedMemories(context.Background(), []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryRepository_touchRetrievedMemories_error(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	mock.ExpectExec(`access_count = access_count \+ 1`).
		WithArgs([]int64{1}).
		WillReturnError(errors.New("db down"))

	repo := &MemoryRepository{w: mock, r: mock}
	if err := repo.touchRetrievedMemories(context.Background(), []int64{1}); err == nil {
		t.Fatal("expected error")
	}
}
