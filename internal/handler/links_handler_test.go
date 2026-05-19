package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

func TestLinksHandlerPost_invalidJSON(t *testing.T) {
	app := newTestApp("tid", "admin")
	h := NewLinksHandler(nil, nil)
	app.Post("/v1/memories/links", h.Post)

	req := httptest.NewRequest("POST", "/v1/memories/links", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestLinksHandlerList_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	now := time.Unix(1700000000, 0).UTC()
	rows := pgxmock.NewRows([]string{
		"id", "from_path", "to_path", "link_type", "metadata", "created_at",
	}).AddRow(int64(7), "root.a", "root.b", "related", []byte("{}"), now)

	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenantID, 50).
		WillReturnRows(rows)

	app := newTestApp(tenantID, "admin")
	h := &LinksHandler{repo: repository.NewLinksRepositoryReadOnly(mock)}
	app.Get("/links", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/links", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Entries []model.MemoryLink `json:"entries"`
		Total   int                `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Entries) != 1 || body.Entries[0].ID != 7 {
		t.Fatalf("unexpected body %+v", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLinksHandlerList_emptyUsesEmptySlice(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	rows := pgxmock.NewRows([]string{
		"id", "from_path", "to_path", "link_type", "metadata", "created_at",
	})
	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenantID, 10).
		WillReturnRows(rows)

	app := newTestApp(tenantID, "admin")
	h := &LinksHandler{repo: repository.NewLinksRepositoryReadOnly(mock)}
	app.Get("/links", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/links?limit=10", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Entries []model.MemoryLink `json:"entries"`
		Total   int                `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 || len(body.Entries) != 0 {
		t.Fatalf("expected empty entries, got %+v", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLinksHandlerList_repositoryError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	mock.ExpectQuery(`SELECT id, from_path`).
		WithArgs(tenantID, 50).
		WillReturnError(errors.New("db unavailable"))

	app := newTestApp(tenantID, "admin")
	h := &LinksHandler{repo: repository.NewLinksRepositoryReadOnly(mock)}
	app.Get("/links", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/links", nil))
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
