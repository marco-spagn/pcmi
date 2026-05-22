package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

func TestLinksRepository_Count_pgxmock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memory_links`).
		WithArgs(tenant).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(42))

	repo := NewLinksRepositoryReadOnly(mock)
	n, err := repo.Count(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if n != 42 {
		t.Fatalf("count: got %d want 42", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLinksRepository_List_toPathAndLinkTypeFilters(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	now := time.Unix(1700000001, 0).UTC()
	rows := pgxmock.NewRows([]string{
		"id", "from_path", "to_path", "link_type", "metadata", "created_at",
	}).AddRow(int64(2), "root.a", "root.b", "depends_on", []byte("{}"), now)

	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenant, "root.a", "root.b", "depends_on", 10).
		WillReturnRows(rows)

	repo := NewLinksRepositoryReadOnly(mock)
	got, err := repo.List(context.Background(), tenant, "root.a", "root.b", "depends_on", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LinkType != "depends_on" {
		t.Fatalf("unexpected %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
