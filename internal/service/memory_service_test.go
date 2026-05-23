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

// ─── Shared mock repo (extends the one in batch_service_test.go) ─────────────

type fullMockRepo struct {
	storeFn              func(req model.StoreRequest) (int64, int, *int64, error)
	getByPathFn          func(path string) (*model.MemoryEntry, error)
	getHistoricalFn      func(path string) (*model.MemoryEntry, error)
	compactFn            func() (int, error)
	exportFn             func() ([]model.MemoryEntry, error)
	updateImportanceFn   func(importance float64) error
}

func (r *fullMockRepo) Store(_ context.Context, req model.StoreRequest, _ string) (int64, int, *int64, error) {
	if r.storeFn != nil {
		return r.storeFn(req)
	}
	return 1, 1, nil, nil
}

func (r *fullMockRepo) Retrieve(_ context.Context, req model.RetrieveRequest, _ string, _ []float32) ([]model.MemoryEntry, error) {
	return []model.MemoryEntry{{ID: 1, Path: req.PathPrefix}}, nil
}

func (r *fullMockRepo) GetByPath(_ context.Context, _ string, path string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	if r.getByPathFn != nil {
		return r.getByPathFn(path)
	}
	return &model.MemoryEntry{ID: 1, Path: path}, nil
}

func (r *fullMockRepo) GetHistoricalVersion(_ context.Context, _ string, path string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	if r.getHistoricalFn != nil {
		return r.getHistoricalFn(path)
	}
	return nil, errors.New("no historical version")
}

func (r *fullMockRepo) ExportMemories(_ context.Context, _ string, _ string, _ int, _ bool) ([]model.MemoryEntry, error) {
	if r.exportFn != nil {
		return r.exportFn()
	}
	return []model.MemoryEntry{{ID: 1, Path: "root.a"}}, nil
}

func (r *fullMockRepo) CompactPathHistory(_ context.Context, _ string, _ string, _ int) (int, error) {
	if r.compactFn != nil {
		return r.compactFn()
	}
	return 0, nil
}

func (r *fullMockRepo) UpdateImportance(_ context.Context, _, _ string, importance float64) error {
	if r.updateImportanceFn != nil {
		return r.updateImportanceFn(importance)
	}
	return nil
}

func (r *fullMockRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return model.DedupModeNone, nil
}
func (r *fullMockRepo) FindCurrentByContentHash(context.Context, string, string) (*model.MemoryEntry, error) {
	return nil, nil
}
func (r *fullMockRepo) MergeCurrentMetadata(context.Context, string, string, map[string]interface{}, []string) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (r *fullMockRepo) UpsertDedupLink(context.Context, string, string, string) error {
	return nil
}

// ─── Store ────────────────────────────────────────────────────────────────────

func TestMemoryServiceStoreSuccess(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &fullMockRepo{}
	svc := NewMemoryService(repo, nil)
	req := &model.StoreRequest{Path: "root.test.foo", Content: "hello"}
	result, err := svc.Store(context.Background(), req, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Entry.Path != "root.test.foo" {
		t.Fatalf("unexpected path: %s", result.Entry.Path)
	}
	if result.Version != 1 {
		t.Fatalf("expected version 1, got %d", result.Version)
	}
}

func TestMemoryServiceStoreRepoError(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &fullMockRepo{
		storeFn: func(_ model.StoreRequest) (int64, int, *int64, error) {
			return 0, 0, nil, errors.New("db down")
		},
	}
	svc := NewMemoryService(repo, nil)
	_, err := svc.Store(context.Background(), &model.StoreRequest{Path: "x", Content: "y"}, "tid")
	if err == nil {
		t.Fatal("expected error from failing repo")
	}
}

func TestMemoryServiceStoreSuperseded(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	supID := int64(99)
	repo := &fullMockRepo{
		storeFn: func(_ model.StoreRequest) (int64, int, *int64, error) {
			return 2, 2, &supID, nil
		},
	}
	svc := NewMemoryService(repo, nil)
	result, err := svc.Store(context.Background(), &model.StoreRequest{Path: "p", Content: "c"}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if result.SupersededID == nil || *result.SupersededID != 99 {
		t.Fatalf("expected SupersededID=99, got %v", result.SupersededID)
	}
}

// ─── Retrieve ─────────────────────────────────────────────────────────────────

func TestMemoryServiceRetrieve(t *testing.T) {
	repo := &fullMockRepo{}
	svc := NewMemoryService(repo, nil)
	resp, err := svc.Retrieve(context.Background(), &model.RetrieveRequest{PathPrefix: "root"}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
}

// ─── GetByPath ────────────────────────────────────────────────────────────────

func TestMemoryServiceGetByPathNotFound(t *testing.T) {
	repo := &fullMockRepo{
		getByPathFn: func(_ string) (*model.MemoryEntry, error) {
			return nil, errors.New("memory not found")
		},
	}
	svc := NewMemoryService(repo, nil)
	_, err := svc.GetByPath(context.Background(), "tid", "missing", nil, nil)
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

// ─── Rollback ─────────────────────────────────────────────────────────────────

func TestMemoryServiceRollbackEmptyPath(t *testing.T) {
	svc := NewMemoryService(&fullMockRepo{}, nil)
	v := 1
	_, err := svc.Rollback(context.Background(), &model.RollbackRequest{Path: "", Version: &v}, "tid")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestMemoryServiceRollbackNoHistory(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &fullMockRepo{
		getHistoricalFn: func(_ string) (*model.MemoryEntry, error) {
			return nil, errors.New("no historical version found")
		},
	}
	svc := NewMemoryService(repo, nil)
	v := 1
	_, err := svc.Rollback(context.Background(), &model.RollbackRequest{Path: "root.x", Version: &v}, "tid")
	if err == nil {
		t.Fatal("expected error when no historical version exists")
	}
}

// ─── Compact ──────────────────────────────────────────────────────────────────

func TestMemoryServiceCompactDefaultKeep(t *testing.T) {
	deleted := 0
	repo := &fullMockRepo{
		compactFn: func() (int, error) { return deleted, nil },
	}
	svc := NewMemoryService(repo, nil)
	resp, err := svc.Compact(context.Background(), "tid", &model.CompactMemoryRequest{Path: "root.x", KeepSuperseded: 0})
	if err != nil {
		t.Fatal(err)
	}
	if resp.KeepSuperseded != 20 {
		t.Fatalf("expected default keep=20, got %d", resp.KeepSuperseded)
	}
}

func TestMemoryServiceCompactCustomKeep(t *testing.T) {
	repo := &fullMockRepo{compactFn: func() (int, error) { return 3, nil }}
	svc := NewMemoryService(repo, nil)
	resp, err := svc.Compact(context.Background(), "tid", &model.CompactMemoryRequest{Path: "root.x", KeepSuperseded: 5})
	if err != nil {
		t.Fatal(err)
	}
	if resp.DeletedCount != 3 {
		t.Fatalf("expected deleted=3, got %d", resp.DeletedCount)
	}
	if resp.KeepSuperseded != 5 {
		t.Fatalf("expected keep=5, got %d", resp.KeepSuperseded)
	}
}

// ─── UpdateImportance ─────────────────────────────────────────────────────────

func TestMemoryServiceUpdateImportance(t *testing.T) {
	var got float64
	repo := &fullMockRepo{
		updateImportanceFn: func(importance float64) error {
			got = importance
			return nil
		},
	}
	svc := NewMemoryService(repo, nil)
	if err := svc.UpdateImportance(context.Background(), "tid", "root.note", 0.75); err != nil {
		t.Fatal(err)
	}
	if got != 0.75 {
		t.Fatalf("importance=%v", got)
	}
}

func TestMemoryServiceUpdateImportance_invalid(t *testing.T) {
	svc := NewMemoryService(&fullMockRepo{}, nil)
	err := svc.UpdateImportance(context.Background(), "tid", "root.note", 2.0)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// ─── Export ───────────────────────────────────────────────────────────────────

func TestMemoryServiceExportDefaultLimit(t *testing.T) {
	repo := &fullMockRepo{}
	svc := NewMemoryService(repo, nil)
	resp, err := svc.Export(context.Background(), "tid", &model.MemoryExportRequest{PathPrefix: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Exported != 1 {
		t.Fatalf("expected 1 exported, got %d", resp.Exported)
	}
}

func TestMemoryServiceExportError(t *testing.T) {
	repo := &fullMockRepo{
		exportFn: func() ([]model.MemoryEntry, error) { return nil, errors.New("export fail") },
	}
	svc := NewMemoryService(repo, nil)
	_, err := svc.Export(context.Background(), "tid", &model.MemoryExportRequest{})
	if err == nil {
		t.Fatal("expected export error")
	}
}
