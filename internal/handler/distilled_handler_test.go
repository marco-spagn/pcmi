package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/gofiber/fiber/v2"
)

func TestDistilledHandler_missingTenantContext(t *testing.T) {
	app := fiber.New()
	app.Get("/v1/distilled", NewDistilledHandler(nil).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled?path_prefix=root.p", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}

func TestDistilledHandler_missingPathPrefix(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	app.Get("/v1/distilled", NewDistilledHandler(nil).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestDistilledHandler_queryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").String()
	mock.ExpectQuery(`SELECT id, path`).
		WithArgs(tenant, "root.p", 51).
		WillReturnError(errors.New("query failed"))

	app := newTestApp(tenant, "admin")
	app.Get("/v1/distilled", NewDistilledHandler(mock).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled?path_prefix=root.p", nil))
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

func TestDistilledHandler_limitClampedHigh(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	rows := pgxmock.NewRows([]string{
		"id", "path", "summary", "insights", "confidence_score", "distilled_at", "source_entry_ids", "version",
	})
	mock.ExpectQuery(`SELECT id, path`).
		WithArgs(tenant, "root", 201).
		WillReturnRows(rows)

	app := newTestApp(tenant, "admin")
	app.Get("/v1/distilled", NewDistilledHandler(mock).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled?path_prefix=root&limit=9999", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 {
		t.Fatalf("total %d want 0", body.Total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDistilledHandler_successRow(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenant := uuid.New().String()
	at := time.Unix(1700000001, 0).UTC()
	rows := pgxmock.NewRows([]string{
		"id", "path", "summary", "insights", "confidence_score", "distilled_at", "source_entry_ids", "version",
	}).AddRow(int64(3), "root.topic", "brief", []byte(`[1,2]`), sql.NullFloat64{Float64: 0.7, Valid: true}, at, []int64{10, 20}, 4)

	mock.ExpectQuery(`SELECT id, path`).
		WithArgs(tenant, "root.topic", 2).
		WillReturnRows(rows)

	app := newTestApp(tenant, "admin")
	app.Get("/v1/distilled", NewDistilledHandler(mock).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled?path_prefix=root.topic&limit=0", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Entries []map[string]any `json:"entries"`
		Limit   int              `json:"limit"`
		Tenant  string           `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 1 || body.Tenant != tenant || body.Limit != 1 {
		t.Fatalf("unexpected %+v", body)
	}
	e0 := body.Entries[0]
	if e0["id"].(float64) != 3 { // JSON numbers decode as float64
		t.Fatalf("id %+v", e0["id"])
	}
	if _, ok := e0["confidence_score"]; !ok {
		t.Fatal("expected confidence_score")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
