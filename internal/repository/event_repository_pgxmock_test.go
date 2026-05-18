package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

func TestEventRepository_Insert_pgxmock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	evID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ts := time.Unix(1700000000, 0).UTC()
	agentID := "33333333-3333-3333-3333-333333333333"

	rows := pgxmock.NewRows([]string{"id", "timestamp"}).
		AddRow(evID.String(), ts)

	mock.ExpectQuery(`INSERT INTO events`).
		WithArgs(
			tenantID.String(),
			"session.started",
			pgxmock.AnyArg(),
			pgxmock.AnyArg(),
		).
		WillReturnRows(rows)

	repo := NewEventRepository(mock)
	id, gotTS, err := repo.Insert(context.Background(), tenantID.String(), "session.started", map[string]interface{}{"a": float64(1)}, &agentID)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if id != evID.String() {
		t.Fatalf("id=%s", id)
	}
	if !gotTS.Equal(ts) {
		t.Fatalf("ts=%v want %v", gotTS, ts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEventRepository_Insert_pgxmockNilAgent(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	evID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	ts := time.Unix(1700000001, 0).UTC()
	tenantID := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	rows := pgxmock.NewRows([]string{"id", "timestamp"}).
		AddRow(evID.String(), ts)

	mock.ExpectQuery(`INSERT INTO events`).
		WithArgs(
			tenantID.String(),
			"tool.called",
			pgxmock.AnyArg(),
			pgxmock.AnyArg(),
		).
		WillReturnRows(rows)

	repo := NewEventRepository(mock)
	if _, _, err := repo.Insert(context.Background(), tenantID.String(), "tool.called", map[string]interface{}{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEventRepository_Insert_pgxmockQueryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`INSERT INTO events`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))

	repo := NewEventRepository(mock)
	if _, _, err := repo.Insert(context.Background(), uuid.New().String(), "x", map[string]interface{}{}, nil); err == nil {
		t.Fatal("expected error")
	}
}
