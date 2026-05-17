package service

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
)

// ─── BatchRetrieve ────────────────────────────────────────────────────────────

func TestBatchRetrieveEmptyQueries(t *testing.T) {
	svc := NewMemoryService(&mockMemoryRepo{}, nil)
	_, err := svc.BatchRetrieve(context.Background(), &model.BatchRetrieveRequest{Queries: nil}, "tid")
	if err == nil {
		t.Fatal("expected error for empty queries")
	}
}

func TestBatchRetrieveTooManyQueries(t *testing.T) {
	queries := make([]model.RetrieveRequest, 21)
	svc := NewMemoryService(&mockMemoryRepo{}, nil)
	_, err := svc.BatchRetrieve(context.Background(), &model.BatchRetrieveRequest{Queries: queries}, "tid")
	if err == nil {
		t.Fatal("expected error for >20 queries")
	}
}

func TestBatchRetrieveSuccess(t *testing.T) {
	svc := NewMemoryService(&mockMemoryRepo{}, nil)
	queries := []model.RetrieveRequest{
		{PathPrefix: "root.a"},
		{PathPrefix: "root.b"},
	}
	resp, err := svc.BatchRetrieve(context.Background(), &model.BatchRetrieveRequest{Queries: queries}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
}

// ─── BatchStore ───────────────────────────────────────────────────────────────

func TestBatchStoreEmptyItems(t *testing.T) {
	svc := NewMemoryService(&mockMemoryRepo{}, nil)
	_, err := svc.BatchStore(context.Background(), &model.BatchStoreRequest{Items: nil}, "tid")
	if err == nil {
		t.Fatal("expected error for empty items")
	}
}

func TestBatchStoreTooManyItems(t *testing.T) {
	items := make([]model.StoreRequest, 51)
	svc := NewMemoryService(&mockMemoryRepo{}, nil)
	_, err := svc.BatchStore(context.Background(), &model.BatchStoreRequest{Items: items}, "tid")
	if err == nil {
		t.Fatal("expected error for >50 items")
	}
}

func TestBatchStoreAllSuccess(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	svc := NewMemoryService(&mockMemoryRepo{}, nil)
	items := []model.StoreRequest{
		{Path: "root.a", Content: "c1"},
		{Path: "root.b", Content: "c2"},
	}
	result, err := svc.BatchStore(context.Background(), &model.BatchStoreRequest{Items: items}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 {
		t.Fatalf("expected total=2, got %d", result.Total)
	}
	for i, r := range result.Results {
		if r.Status != "stored" {
			t.Fatalf("result[%d] status=%s want stored", i, r.Status)
		}
	}
}

// ─── Import ───────────────────────────────────────────────────────────────────

func TestImportNewEntrySkipMode(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	// GetByPath returns "not found" → entry is stored
	repo := &fullMockRepo{
		getByPathFn: func(_ string) (*model.MemoryEntry, error) {
			return nil, errors.New("memory not found")
		},
	}
	svc := NewMemoryService(repo, nil)
	req := &model.MemoryImportRequest{
		Mode: "skip",
		Entries: []model.StoreRequest{
			{Path: "root.new", Content: "hello"},
		},
	}
	resp, err := svc.Import(context.Background(), "tid", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Imported != 1 {
		t.Fatalf("expected 1 imported, got %d", resp.Imported)
	}
}

func TestImportOverwriteMode(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	// overwrite mode: never calls GetByPath, always stores
	repo := &fullMockRepo{}
	svc := NewMemoryService(repo, nil)
	req := &model.MemoryImportRequest{
		Mode: "overwrite",
		Entries: []model.StoreRequest{
			{Path: "root.x", Content: "v1"},
			{Path: "root.y", Content: "v2"},
		},
	}
	resp, err := svc.Import(context.Background(), "tid", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d", resp.Imported)
	}
}

func TestImportDefaultModeIsSkip(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	// No mode set → defaults to "skip". GetByPath returns "exists" path → skip.
	repo := &fullMockRepo{
		getByPathFn: func(path string) (*model.MemoryEntry, error) {
			return &model.MemoryEntry{ID: 1, Path: path}, nil
		},
	}
	svc := NewMemoryService(repo, nil)
	req := &model.MemoryImportRequest{
		Entries: []model.StoreRequest{{Path: "root.exists", Content: "x"}},
	}
	resp, err := svc.Import(context.Background(), "tid", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", resp.Skipped)
	}
}
