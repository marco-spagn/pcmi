package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestIdempotency_CacheCleanup_RemovesExpired(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectExec(`DELETE FROM idempotency_cache WHERE expires_at <= NOW\(\)`).
		WillReturnResult(pgxmock.NewResult("DELETE", 3))

	repo := NewIdempotencyRepository(mock)
	n, err := repo.DeleteExpired(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("deleted %d want 3", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIdempotencyRepository_Get_miss(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	mock.ExpectQuery(`SELECT response_json`).
		WithArgs(tenant, "key-1").
		WillReturnError(pgx.ErrNoRows)

	repo := NewIdempotencyRepository(mock)
	_, ok, err := repo.Get(context.Background(), tenant, "key-1")
	if err != nil || ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}

func TestIdempotencyRepository_Put_upsert(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	body := json.RawMessage(`{"id":1}`)
	mock.ExpectExec(`INSERT INTO idempotency_cache`).
		WithArgs("key-1", tenant, body, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := NewIdempotencyRepository(mock)
	if err := repo.Put(context.Background(), tenant, "key-1", body); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIdempotencyRepository_Get_hit(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	raw := []byte(`{"id":7}`)
	mock.ExpectQuery(`SELECT response_json`).
		WithArgs(tenant, "key-1").
		WillReturnRows(pgxmock.NewRows([]string{"response_json"}).AddRow(raw))

	repo := NewIdempotencyRepository(mock)
	got, ok, err := repo.Get(context.Background(), tenant, "key-1")
	if err != nil || !ok || string(got) != string(raw) {
		t.Fatalf("got=%q ok=%v err=%v", got, ok, err)
	}
}
