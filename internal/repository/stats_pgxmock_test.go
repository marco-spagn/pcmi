package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestStatsRepository_TenantStats_pgxmock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	rows := pgxmock.NewRows([]string{
		"active", "superseded", "distilled", "links", "events", "expiring",
	}).AddRow(10, 2, 3, 4, 5, 1)

	mock.ExpectQuery(`SELECT`).
		WithArgs(tenantID).
		WillReturnRows(rows)

	repo := &StatsRepository{r: mock}
	got, err := repo.TenantStats(context.Background(), tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveMemories != 10 || got.SupersededMemories != 2 || got.DistilledCount != 3 ||
		got.LinksCount != 4 || got.EventsCount != 5 || got.ExpiringSoon != 1 {
		t.Fatalf("unexpected stats: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatsRepository_TenantStats_queryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`SELECT`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	repo := &StatsRepository{r: mock}
	if _, err := repo.TenantStats(context.Background(), uuid.New().String()); err == nil {
		t.Fatal("expected error")
	}
}
