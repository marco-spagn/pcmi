package graph

import "testing"

func TestFindMemoriesByEntity_RequiresKindKey(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindMemoriesByEntity(t.Context(), "tenant", "", "10.0.0.1", 0, 50)
	if err == nil {
		t.Fatal("expected error for empty kind")
	}
}

func TestReconcileEntityMentions_NilDB(t *testing.T) {
	gc := NewGraphClient(nil)
	err := gc.ReconcileEntityMentions(t.Context(), "tenant", 1, 1, nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestFindEntitiesForMemory_AGENotAvailable(t *testing.T) {
	gc := NewGraphClient(nil)
	out, err := gc.FindEntitiesForMemory(t.Context(), "tenant", 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %v", out)
	}
}

func TestFindRelatedViaEntity_AGENotAvailable(t *testing.T) {
	gc := NewGraphClient(nil)
	out, err := gc.FindRelatedViaEntity(t.Context(), "tenant", 42, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.Memories) != 0 {
		t.Fatalf("expected empty result, got %+v", out)
	}
}

func TestPaginateEntityMemories(t *testing.T) {
	seen := map[int64]EntityMemory{
		1: {ID: 1},
		5: {ID: 5},
		10: {ID: 10},
	}
	page := paginateEntityMemories(seen, 0, 2)
	if len(page.Memories) != 2 || page.Total != 3 || page.NextCursor != 5 {
		t.Fatalf("unexpected page: %+v", page)
	}
}
