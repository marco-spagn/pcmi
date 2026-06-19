package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
)

// anyArgs returns n positional AnyArg matchers for pgxmock WithArgs.
func anyArgs(n int) []any {
	args := make([]any, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

// TestStore_RetriesOnOpenVersionConflict simulates a concurrent writer that
// closed the current row first: the first attempt inserts version=1 and hits a
// unique_violation on the open-version index. Store must retry, re-read the
// row the winner left open, and store the correct next version.
func TestStore_RetriesOnOpenVersionConflict(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()

	// Attempt 1: no current row to close → version=1 → INSERT collides.
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE memory_entries`).WithArgs(anyArgs(2)...).
		WillReturnRows(pgxmock.NewRows([]string{"id", "version"})) // empty → ErrNoRows
	mock.ExpectQuery(`INSERT INTO memory_entries`).WithArgs(anyArgs(13)...).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: openVersionConstraint})
	mock.ExpectRollback()

	// Attempt 2: the winner's row (id=5, version=2) is now closable → version=3.
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE memory_entries`).WithArgs(anyArgs(2)...).
		WillReturnRows(pgxmock.NewRows([]string{"id", "version"}).AddRow(int64(5), int64(2)))
	mock.ExpectQuery(`INSERT INTO memory_entries`).WithArgs(anyArgs(13)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectCommit()

	repo := NewMemoryRepositoryFromDB(mock, mock)
	id, version, superseded, err := repo.Store(context.Background(),
		model.StoreRequest{Path: "root.demo.note", Content: "hello"}, tenantID)
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
	if version != 3 {
		t.Fatalf("version = %d, want 3 (recomputed after retry)", version)
	}
	if superseded == nil || *superseded != 5 {
		t.Fatalf("supersededID = %v, want 5", superseded)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestStore_DoesNotRetryOnUnrelatedError ensures the retry is scoped to the
// open-version conflict only: any other insert error is returned immediately
// after a single attempt.
func TestStore_DoesNotRetryOnUnrelatedError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE memory_entries`).WithArgs(anyArgs(2)...).
		WillReturnRows(pgxmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(`INSERT INTO memory_entries`).WithArgs(anyArgs(13)...).
		WillReturnError(errors.New("connection reset"))
	mock.ExpectRollback()

	repo := NewMemoryRepositoryFromDB(mock, mock)
	_, _, _, err = repo.Store(context.Background(),
		model.StoreRequest{Path: "root.demo.note", Content: "hello"}, tenantID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations (a retry would leave a second Begin pending): %v", err)
	}
}
