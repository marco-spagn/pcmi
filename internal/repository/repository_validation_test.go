package repository

import (
	"context"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestMemoryRepository_Store_emptyPath(t *testing.T) {
	r := &MemoryRepository{}
	_, _, _, err := r.Store(context.Background(), model.StoreRequest{Path: "  "}, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMemoryRepository_GetByPath_emptyPath(t *testing.T) {
	r := &MemoryRepository{}
	_, err := r.GetByPath(context.Background(), "00000000-0000-0000-0000-000000000000", "", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetHistoricalVersion_validation(t *testing.T) {
	r := &MemoryRepository{}
	ctx := context.Background()
	tid := "00000000-0000-0000-0000-000000000000"
	if _, err := r.GetHistoricalVersion(ctx, tid, "", nil, nil); err == nil {
		t.Fatal("empty path")
	}
	if _, err := r.GetHistoricalVersion(ctx, tid, "a.b", nil, nil); err == nil {
		t.Fatal("missing version and as_of")
	}
	v := 1
	now := time.Now()
	if _, err := r.GetHistoricalVersion(ctx, tid, "a.b", &v, &now); err == nil {
		t.Fatal("both version and as_of")
	}
}

func TestListPathHistory_emptyPath(t *testing.T) {
	r := &MemoryRepository{}
	_, err := r.ListPathHistory(context.Background(), "00000000-0000-0000-0000-000000000000", "", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCompactPathHistory_emptyPath(t *testing.T) {
	r := &MemoryRepository{}
	_, err := r.CompactPathHistory(context.Background(), "00000000-0000-0000-0000-000000000000", "  ", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLinksRepository_Create_validation(t *testing.T) {
	r := &LinksRepository{}
	_, err := r.Create(context.Background(), "00000000-0000-0000-0000-000000000000", model.CreateLinkRequest{FromPath: "", ToPath: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEventRepository_Insert_emptyType(t *testing.T) {
	r := &EventRepository{}
	_, _, err := r.Insert(context.Background(), "00000000-0000-0000-0000-000000000000", "", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLineageRepository_MemoryLineage_emptyPath(t *testing.T) {
	r := &LineageRepository{}
	_, err := r.MemoryLineage(context.Background(), "00000000-0000-0000-0000-000000000000", "")
	if err == nil {
		t.Fatal("expected error")
	}
}
