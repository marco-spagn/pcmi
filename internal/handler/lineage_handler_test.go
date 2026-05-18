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

type stubLineage struct {
	memResp *model.MemoryLineageResponse
	memErr  error
	disResp *model.DistilledLineageResponse
	disErr  error
}

func (s *stubLineage) MemoryLineage(_ context.Context, _, _ string) (*model.MemoryLineageResponse, error) {
	return s.memResp, s.memErr
}

func (s *stubLineage) DistilledLineage(_ context.Context, _ string, _ int64) (*model.DistilledLineageResponse, error) {
	return s.disResp, s.disErr
}

func TestLineageHandler_MemoryLineage_missingPath(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{}}
	app.Get("/v1/lineage/memory", h.MemoryLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/memory", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestLineageHandler_MemoryLineage_notFound(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{memErr: errors.New("memory not found")}}
	app.Get("/v1/lineage/memory", h.MemoryLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/memory?path=root.x", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d want 404", resp.StatusCode)
	}
}

func TestLineageHandler_MemoryLineage_internalError(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{memErr: errors.New("db unavailable")}}
	app.Get("/v1/lineage/memory", h.MemoryLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/memory?path=root.x", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
}

func TestLineageHandler_MemoryLineage_success(t *testing.T) {
	t.Parallel()
	payload := &model.MemoryLineageResponse{
		EntryID:  42,
		Path:     "root.a",
		Versions: []model.MemoryEntry{{ID: 42, Path: "root.a", Version: 1}},
		Distilled: []model.DistilledLineageItem{
			{ID: 7, Path: "root.a", Summary: "d"},
		},
	}
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{memResp: payload}}
	app.Get("/v1/lineage/memory", h.MemoryLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/memory?path=root.a", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var got model.MemoryLineageResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.EntryID != 42 || len(got.Versions) != 1 || len(got.Distilled) != 1 {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestLineageHandler_DistilledLineage_invalidID(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{}}
	app.Get("/v1/lineage/distilled/:id", h.DistilledLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/distilled/notnum", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestLineageHandler_DistilledLineage_notFound(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{disErr: errors.New("distilled not found")}}
	app.Get("/v1/lineage/distilled/:id", h.DistilledLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/distilled/99", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d want 404", resp.StatusCode)
	}
}

func TestLineageHandler_DistilledLineage_success(t *testing.T) {
	t.Parallel()
	body := &model.DistilledLineageResponse{
		Distilled: model.DistilledLineageItem{ID: 9, Path: "p", Summary: "s", Version: 2},
		Sources:   []model.MemoryEntry{{ID: 1, Content: "c"}},
	}
	app := newTestApp(uuid.New().String(), "admin")
	h := &LineageHandler{repo: &stubLineage{disResp: body}}
	app.Get("/v1/lineage/distilled/:id", h.DistilledLineage)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/lineage/distilled/9", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var got model.DistilledLineageResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Distilled.ID != 9 || len(got.Sources) != 1 {
		t.Fatalf("unexpected %+v", got)
	}
}
