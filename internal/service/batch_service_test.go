package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
)

type mockMemoryRepo struct {
	storeCalls int
}

func (m *mockMemoryRepo) Store(ctx context.Context, req model.StoreRequest, tenantID string) (int64, int, *int64, error) {
	m.storeCalls++
	if req.Path == "fail" {
		return 0, 0, nil, errors.New("boom")
	}
	id := int64(m.storeCalls)
	return id, m.storeCalls, nil, nil
}

func (m *mockMemoryRepo) Retrieve(ctx context.Context, req model.RetrieveRequest, tenantID string, emb []float32) ([]model.MemoryEntry, error) {
	return []model.MemoryEntry{{ID: 1, Path: req.PathPrefix}}, nil
}

func (m *mockMemoryRepo) GetHistoricalVersion(ctx context.Context, tenantID, path string, version *int, asOf *time.Time) (*model.MemoryEntry, error) {
	return nil, errors.New("not found")
}

func (m *mockMemoryRepo) GetByPath(ctx context.Context, tenantID, path string, version *int, asOf *time.Time) (*model.MemoryEntry, error) {
	if path == "exists" {
		return &model.MemoryEntry{ID: 1, Path: path}, nil
	}
	return nil, errors.New("memory not found")
}

func (m *mockMemoryRepo) GetByIDResolveCurrent(_ context.Context, _ string, memoryID int64) (*model.MemoryEntry, int64, error) {
	if memoryID == 1 {
		return &model.MemoryEntry{ID: 1, Path: "exists"}, memoryID, nil
	}
	return nil, memoryID, errors.New("memory not found")
}

func (m *mockMemoryRepo) ExportMemories(ctx context.Context, tenantID, pathPrefix string, limit int, includeEmb bool) ([]model.MemoryEntry, error) {
	return []model.MemoryEntry{{ID: 1, Path: "root.a"}}, nil
}

func (m *mockMemoryRepo) CompactPathHistory(ctx context.Context, tenantID, path string, keepSuperseded int) (int, error) {
	return 0, nil
}

func (m *mockMemoryRepo) UpdateImportance(ctx context.Context, tenantID, path string, importance float64) error {
	return nil
}

func (m *mockMemoryRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return model.DedupModeNone, nil
}
func (m *mockMemoryRepo) FindCurrentByContentHash(context.Context, string, string) (*model.MemoryEntry, error) {
	return nil, nil
}
func (m *mockMemoryRepo) MergeCurrentMetadata(context.Context, string, string, map[string]interface{}, []string) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockMemoryRepo) UpsertDedupLink(context.Context, string, string, string) error {
	return nil
}

func TestBatchStorePartialSuccess(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &mockMemoryRepo{}
	svc := NewMemoryService(repo, nil)
	res, err := svc.BatchStore(context.Background(), &model.BatchStoreRequest{
		Items: []model.StoreRequest{
			{Path: "ok", Content: "a"},
			{Path: "fail", Content: "b"},
		},
	}, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 || res.Results[0].Status != "stored" || res.Results[1].Status != "error" {
		t.Fatalf("unexpected results: %+v", res.Results)
	}
}

func TestImportSkipExisting(t *testing.T) {
	repo := &mockMemoryRepo{}
	svc := NewMemoryService(repo, nil)
	res, err := svc.Import(context.Background(), "tenant", &model.MemoryImportRequest{
		Mode:    "skip",
		Entries: []model.StoreRequest{{Path: "exists", Content: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Imported != 0 {
		t.Fatalf("expected skip: %+v", res)
	}
}

func TestImportRejectsTooManyEntries(t *testing.T) {
	repo := &mockMemoryRepo{}
	svc := NewMemoryService(repo, nil)

	entries := make([]model.StoreRequest, maxImportEntries+1)
	for i := range entries {
		entries[i] = model.StoreRequest{Path: "root.a", Content: "x"}
	}

	_, err := svc.Import(context.Background(), "tenant", &model.MemoryImportRequest{Entries: entries})
	if err == nil {
		t.Fatal("expected error for oversized import, got nil")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("error should mention the maximum: %v", err)
	}
	if repo.storeCalls != 0 {
		t.Fatalf("import must reject before any Store (fail-fast); storeCalls=%d", repo.storeCalls)
	}
}

func TestImportAtCapIsAccepted(t *testing.T) {
	repo := &mockMemoryRepo{}
	svc := NewMemoryService(repo, nil)

	// Use the skip path (mock GetByPath reports "exists" as present) so exactly
	// maxImportEntries entries are processed without touching the event bus.
	entries := make([]model.StoreRequest, maxImportEntries)
	for i := range entries {
		entries[i] = model.StoreRequest{Path: "exists", Content: "x"}
	}

	res, err := svc.Import(context.Background(), "tenant", &model.MemoryImportRequest{Mode: "skip", Entries: entries})
	if err != nil {
		t.Fatalf("import at the cap should succeed: %v", err)
	}
	if res.Skipped != maxImportEntries {
		t.Fatalf("expected %d skipped, got %d", maxImportEntries, res.Skipped)
	}
}
