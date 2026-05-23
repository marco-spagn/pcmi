package service

import (
	"context"
	"errors"
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
