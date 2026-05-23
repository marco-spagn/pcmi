package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

type summarizeMemStub struct {
	retrieveErr error
	entries     []model.MemoryEntry
}

func (s *summarizeMemStub) Store(context.Context, model.StoreRequest, string) (int64, int, *int64, error) {
	return 0, 0, nil, errors.New("unused")
}

func (s *summarizeMemStub) Retrieve(context.Context, model.RetrieveRequest, string, []float32) ([]model.MemoryEntry, error) {
	if s.retrieveErr != nil {
		return nil, s.retrieveErr
	}
	return s.entries, nil
}

func (s *summarizeMemStub) GetHistoricalVersion(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, nil
}

func (s *summarizeMemStub) GetByPath(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, nil
}

func (s *summarizeMemStub) ExportMemories(context.Context, string, string, int, bool) ([]model.MemoryEntry, error) {
	return nil, nil
}

func (s *summarizeMemStub) CompactPathHistory(context.Context, string, string, int) (int, error) {
	return 0, nil
}

func (s *summarizeMemStub) UpdateImportance(context.Context, string, string, float64) error {
	return nil
}

func (s *summarizeMemStub) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return model.DedupModeNone, nil
}
func (s *summarizeMemStub) FindCurrentByContentHash(context.Context, string, string) (*model.MemoryEntry, error) {
	return nil, nil
}
func (s *summarizeMemStub) MergeCurrentMetadata(context.Context, string, string, map[string]interface{}, []string) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (s *summarizeMemStub) UpsertDedupLink(context.Context, string, string, string) error {
	return nil
}

func TestSummarizeHandler_invalidJSON(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	h := &SummarizeHandler{svc: service.NewSummarizeService(&summarizeMemStub{}, nil)}
	app := newTestApp(uuid.New().String(), "admin")
	app.Post("/summarize", h.Post)

	req := httptest.NewRequest("POST", "/summarize", strings.NewReader("{not-json"))
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

func TestSummarizeHandler_retrieveError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	h := &SummarizeHandler{svc: service.NewSummarizeService(&summarizeMemStub{retrieveErr: errors.New("boom")}, nil)}
	app := newTestApp(uuid.New().String(), "admin")
	app.Post("/summarize", h.Post)

	body := `{"path_prefix":"root","limit":5}`
	req := httptest.NewRequest("POST", "/summarize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
}

func TestSummarizeHandler_noMemories(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	h := &SummarizeHandler{svc: service.NewSummarizeService(&summarizeMemStub{entries: nil}, nil)}
	app := newTestApp(uuid.New().String(), "admin")
	app.Post("/summarize", h.Post)

	body := `{"path_prefix":"root.x","limit":3}`
	req := httptest.NewRequest("POST", "/summarize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var got service.SummarizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Method != "none" || got.Total != 0 || got.Summary != "" {
		t.Fatalf("unexpected %+v", got)
	}
	if got.PathPrefix != "root.x" {
		t.Fatalf("path prefix %q", got.PathPrefix)
	}
}

func TestSummarizeHandler_extractiveWithoutLLM(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	entries := []model.MemoryEntry{
		{ID: 1, Content: "first note"},
		{ID: 2, Content: "second note"},
	}
	h := &SummarizeHandler{svc: service.NewSummarizeService(&summarizeMemStub{entries: entries}, nil)}
	app := newTestApp(uuid.New().String(), "admin")
	app.Post("/summarize", h.Post)

	body := `{"path_prefix":"root","limit":10}`
	req := httptest.NewRequest("POST", "/summarize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var got service.SummarizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Method != "extractive" || got.Total != 2 || got.Summary == "" {
		t.Fatalf("unexpected %+v", got)
	}
	if len(got.SourceIDs) != 2 || got.SourceIDs[0] != 1 || got.SourceIDs[1] != 2 {
		t.Fatalf("source ids %+v", got.SourceIDs)
	}
}
