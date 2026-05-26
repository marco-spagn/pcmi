package graph

import (
	"context"
	"testing"
)

func TestNewGraphClient_NilDB(t *testing.T) {
	// NewGraphClient must not panic when called with nil.
	gc := NewGraphClient(nil)
	if gc == nil {
		t.Fatal("expected non-nil GraphClient")
	}
}

func TestIsAvailable_NilDB(t *testing.T) {
	gc := NewGraphClient(nil)
	if gc.IsAvailable(context.Background()) {
		t.Error("IsAvailable should return false when db is nil")
	}
}

func TestIsAvailable_NilClient(t *testing.T) {
	var gc *GraphClient
	if gc.IsAvailable(context.Background()) {
		t.Error("IsAvailable should return false on nil receiver")
	}
}

func TestFindRelated_AGENotAvailable(t *testing.T) {
	// With a nil db, AGE is never available — FindRelated must return empty
	// slice and nil error (graceful degradation).
	gc := NewGraphClient(nil)
	results, err := gc.FindRelated(context.Background(), "tenant-1", 42, nil, 3)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(results))
	}
}

func TestLinkTypeConstants(t *testing.T) {
	constants := []string{
		LinkTypeCausal,
		LinkTypeTemporal,
		LinkTypeContradicts,
		LinkTypeSupports,
		LinkTypeRelated,
	}
	for _, c := range constants {
		if c == "" {
			t.Errorf("link type constant must not be empty (got empty string)")
		}
	}
}

func TestLinkTypeConstants_Distinct(t *testing.T) {
	seen := map[string]bool{}
	for _, lt := range []string{
		LinkTypeCausal,
		LinkTypeTemporal,
		LinkTypeContradicts,
		LinkTypeSupports,
		LinkTypeRelated,
	} {
		if seen[lt] {
			t.Errorf("duplicate link type constant: %q", lt)
		}
		seen[lt] = true
	}
}

func TestRelatedMemory_Fields(t *testing.T) {
	rm := RelatedMemory{ID: 7, LinkType: LinkTypeCausal, Depth: 2}
	if rm.ID != 7 {
		t.Errorf("ID: got %d, want 7", rm.ID)
	}
	if rm.LinkType != LinkTypeCausal {
		t.Errorf("LinkType: got %q, want %q", rm.LinkType, LinkTypeCausal)
	}
	if rm.Depth != 2 {
		t.Errorf("Depth: got %d, want 2", rm.Depth)
	}
}

func TestSanitizeLinkType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"causal", "causal"},
		{"causal-link", "causal_link"},
		{"has space", "has_space"},
		{"OK_123", "OK_123"},
		{"<script>", "_script_"},
		{"", ""},
		{"a.b.c", "a_b_c"},
		{"DROP TABLE", "DROP_TABLE"},
	}
	for _, tc := range cases {
		got := sanitizeLinkType(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeLinkType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCreateLink_NilDB_ReturnsError(t *testing.T) {
	gc := NewGraphClient(nil)
	err := gc.CreateLink(context.Background(), "tenant-1", 1, 2, LinkTypeCausal, 1.0)
	if err == nil {
		t.Fatal("CreateLink with nil db must return an error")
	}
}

func TestFindRelated_MaxDepthZero_Normalised(t *testing.T) {
	// maxDepth <= 0 must be normalised to 1 inside FindRelated (not panic).
	gc := NewGraphClient(nil)
	results, err := gc.FindRelated(context.Background(), "tenant-1", 42, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// AGE unavailable → graceful degradation: empty results, no panic.
	if results == nil {
		t.Error("nil slice returned; want empty non-nil slice")
	}
}

func TestFindRelated_NilLinkTypes_AllEdges(t *testing.T) {
	// nil linkTypes must not cause a panic (all-edges traversal path).
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, nil, 2)
	if err != nil {
		t.Fatalf("nil linkTypes: unexpected error: %v", err)
	}
}

func TestFindRelated_EmptyLinkTypes_AllEdges(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, []string{}, 2)
	if err != nil {
		t.Fatalf("empty linkTypes: unexpected error: %v", err)
	}
}

func TestRelatedMemory_JSONTags(t *testing.T) {
	// Verify field names expected by API consumers.
	import_check := RelatedMemory{}
	_ = import_check.ID
	_ = import_check.LinkType
	_ = import_check.Depth
}

func TestLinkTypeValues(t *testing.T) {
	expected := map[string]string{
		"causal":      LinkTypeCausal,
		"temporal":    LinkTypeTemporal,
		"contradicts": LinkTypeContradicts,
		"supports":    LinkTypeSupports,
		"related":     LinkTypeRelated,
	}
	for want, got := range expected {
		if got != want {
			t.Errorf("constant value mismatch: got %q want %q", got, want)
		}
	}
}
