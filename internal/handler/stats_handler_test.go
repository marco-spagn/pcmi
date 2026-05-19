package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

func TestStatsRoute_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	rows := pgxmock.NewRows([]string{
		"active", "superseded", "distilled", "links", "events", "expiring",
	}).AddRow(int64(3), int64(1), int64(2), int64(4), int64(5), int64(0))

	mock.ExpectQuery(`SELECT`).
		WithArgs(tenantID).
		WillReturnRows(rows)

	app := newTestApp(tenantID, "admin")
	registerStatsRouteWithRepo(app, repository.NewStatsRepositoryFromRowDB(mock))

	resp, err := app.Test(httptest.NewRequest("GET", "/stats", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var got model.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveMemories != 3 || got.LinksCount != 4 || got.EventsCount != 5 {
		t.Fatalf("unexpected stats %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatsRoute_repositoryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	mock.ExpectQuery(`SELECT`).
		WithArgs(tenantID).
		WillReturnError(pgx.ErrNoRows)

	app := newTestApp(tenantID, "admin")
	registerStatsRouteWithRepo(app, repository.NewStatsRepositoryFromRowDB(mock))

	resp, err := app.Test(httptest.NewRequest("GET", "/stats", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
