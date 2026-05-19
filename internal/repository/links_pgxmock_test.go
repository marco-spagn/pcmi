package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

func TestLinksRepository_List_defaultLimit(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	now := time.Unix(1700000000, 0).UTC()
	rows := pgxmock.NewRows([]string{
		"id", "from_path", "to_path", "link_type", "metadata", "created_at",
	}).AddRow(int64(1), "root.a", "root.b", "related", []byte("{}"), now)

	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenant, 50).
		WillReturnRows(rows)

	repo := NewLinksRepositoryReadOnly(mock)
	got, err := repo.List(context.Background(), tenant, "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FromPath != "root.a" {
		t.Fatalf("unexpected %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLinksRepository_List_clampsHighLimit(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	rows := pgxmock.NewRows([]string{
		"id", "from_path", "to_path", "link_type", "metadata", "created_at",
	})
	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenant, 200).
		WillReturnRows(rows)

	repo := NewLinksRepositoryReadOnly(mock)
	if _, err := repo.List(context.Background(), tenant, "", "", "", 99999); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLinksRepository_List_fromPathFilter(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	rows := pgxmock.NewRows([]string{
		"id", "from_path", "to_path", "link_type", "metadata", "created_at",
	})
	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenant, "root.src", 5).
		WillReturnRows(rows)

	repo := NewLinksRepositoryReadOnly(mock)
	if _, err := repo.List(context.Background(), tenant, "root.src", "", "", 5); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
