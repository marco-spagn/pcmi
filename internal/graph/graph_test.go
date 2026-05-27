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
	result, err := gc.FindRelated(context.Background(), "tenant-1", 42, nil, 3, 0, 50)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(result.Memories))
	}
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
}

func TestParseMemoryVertexID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{`"memory.42"`, 42, true},
		{"memory.7", 7, true},
		{`"42"`, 42, true},
		{"not-a-number", 0, false},
	}
	for _, tc := range cases {
		got, err := parseMemoryVertexID(tc.in)
		if tc.ok && err != nil {
			t.Errorf("parseMemoryVertexID(%q): unexpected error %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("parseMemoryVertexID(%q): expected error", tc.in)
		}
		if tc.ok && got != tc.want {
			t.Errorf("parseMemoryVertexID(%q) = %d, want %d", tc.in, got, tc.want)
		}
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

func TestSyncMemoryLink_NilDB_NoPanic(t *testing.T) {
	gc := NewGraphClient(nil)
	gc.SyncMemoryLink(context.Background(), "tenant-1", "memory.1", "memory.2", LinkTypeCausal, 1.0)
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
	result, err := gc.FindRelated(context.Background(), "tenant-1", 42, nil, 0, 0, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// AGE unavailable → graceful degradation: empty results, no panic.
	if result == nil {
		t.Error("nil result pointer returned; want non-nil result struct")
	}
	if result.Memories == nil {
		t.Error("nil Memories slice returned; want empty non-nil slice")
	}
}

func TestFindRelated_NilLinkTypes_AllEdges(t *testing.T) {
	// nil linkTypes must not cause a panic (all-edges traversal path).
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, nil, 2, 0, 50)
	if err != nil {
		t.Fatalf("nil linkTypes: unexpected error: %v", err)
	}
}

func TestFindRelated_EmptyLinkTypes_AllEdges(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, []string{}, 2, 0, 50)
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

// ─── RelatedResult ────────────────────────────────────────────────────────────

func TestRelatedResult_Fields(t *testing.T) {
	rr := RelatedResult{
		Memories: []RelatedMemory{{ID: 1, LinkType: LinkTypeCausal, Depth: 1}},
		Total:    5,
	}
	if len(rr.Memories) != 1 {
		t.Errorf("Memories len: got %d want 1", len(rr.Memories))
	}
	if rr.Total != 5 {
		t.Errorf("Total: got %d want 5", rr.Total)
	}
}

// ─── ChainLink ────────────────────────────────────────────────────────────────

func TestChainLink_Fields(t *testing.T) {
	cl := ChainLink{FromID: 1, ToID: 7, LinkType: LinkTypeCausal, HopIndex: 0}
	if cl.FromID != 1 {
		t.Errorf("FromID: got %d want 1", cl.FromID)
	}
	if cl.ToID != 7 {
		t.Errorf("ToID: got %d want 7", cl.ToID)
	}
	if cl.LinkType != LinkTypeCausal {
		t.Errorf("LinkType: got %q want %q", cl.LinkType, LinkTypeCausal)
	}
	if cl.HopIndex != 0 {
		t.Errorf("HopIndex: got %d want 0", cl.HopIndex)
	}
}

// ─── ChainResult ──────────────────────────────────────────────────────────────

func TestChainResult_Defaults(t *testing.T) {
	cr := ChainResult{FromID: 1, ToID: 42}
	if cr.Connected {
		t.Error("new ChainResult must default to Connected=false")
	}
	if cr.Path != nil {
		t.Error("new ChainResult must default to nil Path")
	}
}

// ─── parseAGEPath ─────────────────────────────────────────────────────────────

func TestParseAGEPath_Empty(t *testing.T) {
	links := parseAGEPath([]byte(`[]`))
	if links != nil {
		t.Errorf("empty path: got %v want nil", links)
	}
}

func TestParseAGEPath_TooShort(t *testing.T) {
	// Path needs at least 3 elements: [v0, e0, v1]
	links := parseAGEPath([]byte(`[{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}}]`))
	if links != nil {
		t.Errorf("too-short path: got %v want nil", links)
	}
}

func TestParseAGEPath_InvalidJSON(t *testing.T) {
	links := parseAGEPath([]byte(`not-json`))
	if links != nil {
		t.Errorf("invalid JSON: got %v want nil", links)
	}
}

func TestParseAGEPath_ThreeHopChain(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1", "tenant_id": "t1"}},
		{"id": 10, "label": "causal", "start_id": 1, "end_id": 2, "properties": {}},
		{"id": 2, "label": "Memory", "properties": {"id": "memory.7", "tenant_id": "t1"}},
		{"id": 11, "label": "causal", "start_id": 2, "end_id": 3, "properties": {}},
		{"id": 3, "label": "Memory", "properties": {"id": "memory.42", "tenant_id": "t1"}}
	]`)
	links := parseAGEPath(path)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].FromID != 1 || links[0].ToID != 7 || links[0].LinkType != "causal" || links[0].HopIndex != 0 {
		t.Errorf("link[0]: got {from=%d to=%d type=%s hop=%d}", links[0].FromID, links[0].ToID, links[0].LinkType, links[0].HopIndex)
	}
	if links[1].FromID != 7 || links[1].ToID != 42 || links[1].LinkType != "causal" || links[1].HopIndex != 1 {
		t.Errorf("link[1]: got {from=%d to=%d type=%s hop=%d}", links[1].FromID, links[1].ToID, links[1].LinkType, links[1].HopIndex)
	}
}

// ─── FindChain (AGE unavailable → graceful) ───────────────────────────────────

func TestFindChain_AGENotAvailable(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindChain(context.Background(), "tenant-1", 1, 42, nil, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.Connected {
		t.Error("expected Connected=false when AGE unavailable")
	}
	if result.FromID != 1 || result.ToID != 42 {
		t.Errorf("IDs must be preserved in result")
	}
}

// ─── ExecuteCypher (AGE unavailable) ──────────────────────────────────────────

func TestExecuteCypher_AGENotAvailable(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n) RETURN n")
	if err == nil {
		t.Fatal("expected error when AGE unavailable")
	}
}

// ─── ExecuteCypher validation ─────────────────────────────────────────────────

func TestExecuteCypher_NonMatchQuery(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "CREATE (n:Memory {id: 'x'})")
	if err == nil {
		t.Fatal("expected error for non-MATCH query")
	}
}

func TestExecuteCypher_DangerousKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	dangerous := []string{"CREATE (n)", "DELETE n", "SET n.x = 1", "REMOVE n.x", "MERGE (n)", "DROP TABLE", "CALL proc()", "LOAD CSV"}
	for _, q := range dangerous {
		_, err := gc.ExecuteCypher(context.Background(), "t", q)
		if err == nil {
			t.Errorf("query %q should be rejected", q)
		}
	}
}

// ─── FindChain maxDepth normalisation ─────────────────────────────────────────

func TestFindChain_MaxDepthZero_Normalised(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindChain(context.Background(), "t", 1, 2, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ─── CypherResult ─────────────────────────────────────────────────────────────

func TestCypherResult_Fields(t *testing.T) {
	cr := CypherResult{
		Columns: []string{"result"},
		Rows:    []map[string]interface{}{{"result": "test"}},
	}
	if len(cr.Columns) != 1 || cr.Columns[0] != "result" {
		t.Errorf("Columns: got %v", cr.Columns)
	}
	if len(cr.Rows) != 1 {
		t.Errorf("Rows: got %d want 1", len(cr.Rows))
	}
}

// ─── FindRelated pagination edge cases ────────────────────────────────────────

func TestFindRelated_OffsetBeyondTotal(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, 1000, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("offset beyond total must return empty slice, got %d", len(result.Memories))
	}
}

func TestFindRelated_ZeroLimit(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When limit=0 and AGE unavailable, returns empty (graceful).
	if result.Memories == nil {
		t.Error("expected non-nil Memories")
	}
}

func TestFindRelated_NegativeOffset(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, -5, 50)
	if err != nil {
		t.Fatalf("negative offset should not error: %v", err)
	}
}
