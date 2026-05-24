package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/model"
)

type stubPathHistory struct {
	entries []model.MemoryEntry
	page    model.PageResponse
	err     error
}

func (s *stubPathHistory) ListPathHistory(_ context.Context, _, _ string, _ model.PageRequest) ([]model.MemoryEntry, model.PageResponse, error) {
	return s.entries, s.page, s.err
}

func (s *stubPathHistory) CountPathHistory(_ context.Context, _, _ string) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return len(s.entries), nil
}

func TestHistoryHandlerGet_missingPath(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &HistoryHandler{repo: &stubPathHistory{}}
	app.Get("/history", h.Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/history", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestHistoryHandlerGet_repositoryError(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &HistoryHandler{repo: &stubPathHistory{err: errors.New("db down")}}
	app.Get("/history", h.Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/history?path=root.a", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
}

func TestHistoryHandlerGet_success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	entries := []model.MemoryEntry{
		{ID: 1, Path: "root.a", Content: "v2", Version: 2},
		{ID: 2, Path: "root.a", Content: "v1", Version: 1},
	}
	app := newTestApp(tenantID, "admin")
	h := &HistoryHandler{repo: &stubPathHistory{entries: entries}}
	app.Get("/history", h.Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/history?path=root.a&limit=10", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Path    string              `json:"path"`
		Entries []model.MemoryEntry `json:"entries"`
		Total   int                 `json:"total"`
		Limit   int                 `json:"limit"`
		HasMore bool                `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Path != "root.a" || body.Limit != 10 || body.Total != 2 || len(body.Entries) != 2 {
		t.Fatalf("unexpected %+v", body)
	}
}
