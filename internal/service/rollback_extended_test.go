package service

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
)

func TestRollbackByVersion(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &fullMockRepo{
		getHistoricalFn: func(path string) (*model.MemoryEntry, error) {
			return &model.MemoryEntry{
				ID:      42,
				Path:    path,
				Content: "old content",
				Version: 1,
			}, nil
		},
	}
	svc := NewMemoryService(repo, nil)
	v := 1
	resp, err := svc.Rollback(t.Context(), &model.RollbackRequest{Path: "root.x", Version: &v}, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil rollback response")
	}
}

func TestRollbackByAsOf(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &fullMockRepo{
		getHistoricalFn: func(path string) (*model.MemoryEntry, error) {
			return &model.MemoryEntry{
				ID:      10,
				Path:    path,
				Content: "snapshot content",
				Tags:    []string{"tag1"},
			}, nil
		},
	}
	svc := NewMemoryService(repo, nil)
	ts := time.Now().Add(-time.Hour)
	resp, err := svc.Rollback(t.Context(), &model.RollbackRequest{Path: "root.y", AsOf: &ts}, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRollbackWithSourceAgentID(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	agentID := "agent-99"
	repo := &fullMockRepo{
		getHistoricalFn: func(path string) (*model.MemoryEntry, error) {
			return &model.MemoryEntry{
				ID:            1,
				Path:          path,
				Content:       "c",
				SourceAgentID: &agentID,
			}, nil
		},
	}
	svc := NewMemoryService(repo, nil)
	v := 1
	_, err := svc.Rollback(t.Context(), &model.RollbackRequest{Path: "root.z", Version: &v}, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRollbackStoreError(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	repo := &fullMockRepo{
		getHistoricalFn: func(path string) (*model.MemoryEntry, error) {
			return &model.MemoryEntry{ID: 1, Path: path, Content: "c"}, nil
		},
		storeFn: func(_ model.StoreRequest) (int64, int, *int64, error) {
			return 0, 0, nil, errors.New("store down")
		},
	}
	svc := NewMemoryService(repo, nil)
	v := 1
	_, err := svc.Rollback(t.Context(), &model.RollbackRequest{Path: "root.x", Version: &v}, "tid")
	if err == nil {
		t.Fatal("expected error when store fails during rollback")
	}
}
