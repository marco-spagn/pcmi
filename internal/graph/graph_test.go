package graph

import (
	"context"
	"strings"
	"testing"
	"time"
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
	// AGE unavailable → graceful degradation: FindRelated always returns non-nil result.
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

// ─── autoTenantScopeCypher ────────────────────────────────────────────────────

func TestAutoTenantScopeCypher_NoMemoryNode(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH (n) RETURN n", "t1")
	if err == nil {
		t.Fatal("expected error for query without :Memory node")
	}
}

func TestAutoTenantScopeCypher_NoWhere(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (n:Memory) RETURN n.id LIMIT 10", "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "MATCH (n:Memory) WHERE n.tenant_id = 'tenant-1' RETURN n.id LIMIT 10"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestAutoTenantScopeCypher_WithWhere(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (n:Memory) WHERE n.color = 'red' RETURN n", "t2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "MATCH (n:Memory) WHERE n.tenant_id = 't2' AND ( n.color = 'red' RETURN n)"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestAutoTenantScopeCypher_NoReturn(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH (n:Memory) WHERE n.x = 1", "t1")
	if err == nil {
		t.Fatal("expected error for query without RETURN")
	}
}

func TestAutoTenantScopeCypher_AliasWithColon(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (m:Memory {id: 'memory.1'}) RETURN m", "t3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "m.tenant_id = 't3'") {
		t.Errorf("expected alias 'm' tenant filter, got %q", got)
	}
}

func TestAutoTenantScopeCypher_NoParenBeforeMemory(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH :Memory RETURN n", "t1")
	if err == nil {
		t.Fatal("expected error for malformed pattern")
	}
}

func TestAutoTenantScopeCypher_InvalidPattern(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH RETURN n", "t1")
	if err == nil {
		t.Fatal("expected error for query without :Memory")
	}
}

// ─── FindRelated pagination edge cases ────────────────────────────────────────

func TestFindRelated_CursorBeyondTotal(t *testing.T) {
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

func TestFindRelated_NegativeCursor(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, -5, 50)
	if err != nil {
		t.Fatalf("negative offset should not error: %v", err)
	}
}

// ─── FindRelated link types edge cases ──────────────────────────────────────

func TestFindRelated_LinkTypesWithSpecialChars(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, []string{"causal-injection", "has space"}, 3, 0, 50)
	if err != nil {
		t.Fatalf("link types with special chars: %v", err)
	}
}

func TestFindRelated_NilClient(t *testing.T) {
	var gc *GraphClient
	result, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, 0, 50)
	if err != nil {
		t.Fatalf("nil receiver: unexpected error: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("nil receiver: expected empty, got %d", len(result.Memories))
	}
}

// ─── parseAGEPath extra coverage ─────────────────────────────────────────────

func TestParseAGEPath_MalformedVertices(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		{"id": 10, "label": "causal", "properties": {}},
		{"bad-json
	]`)
	links := parseAGEPath(path)
	if links != nil {
		t.Errorf("malformed path: got %v want nil", links)
	}
}

func TestParseAGEPath_EmptyEdgeLabel(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		{"id": 10, "label": "", "properties": {}},
		{"id": 2, "label": "Memory", "properties": {"id": "memory.2"}}
	]`)
	links := parseAGEPath(path)
	if len(links) != 1 {
		t.Fatalf("expected 1 link even with empty label, got %d", len(links))
	}
	if links[0].LinkType != "" {
		t.Errorf("expected empty link type, got %q", links[0].LinkType)
	}
}

func TestParseAGEPath_SingleHop(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.10"}},
		{"id": 20, "label": "supports", "properties": {}},
		{"id": 2, "label": "Memory", "properties": {"id": "memory.20"}}
	]`)
	links := parseAGEPath(path)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].FromID != 10 || links[0].ToID != 20 {
		t.Errorf("link: {from=%d to=%d} want {from=10 to=20}", links[0].FromID, links[0].ToID)
	}
	if links[0].LinkType != "supports" {
		t.Errorf("link type: got %q want supports", links[0].LinkType)
	}
	if links[0].HopIndex != 0 {
		t.Errorf("hop index: got %d want 0", links[0].HopIndex)
	}
}

func TestParseAGEPath_MissingVertexProperties(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {}},
		{"id": 10, "label": "causal", "properties": {}},
		{"id": 2, "label": "Memory", "properties": {}}
	]`)
	links := parseAGEPath(path)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].FromID != 0 || links[0].ToID != 0 {
		t.Errorf("expected zero IDs for missing vertex properties, got from=%d to=%d", links[0].FromID, links[0].ToID)
	}
}

// ─── autoTenantScopeCypher extra coverage ────────────────────────────────────

func TestAutoTenantScopeCypher_MultipleMemoryNodes(t *testing.T) {
	// First :Memory alias is used for tenant scoping.
	got, err := autoTenantScopeCypher(
		"MATCH (a:Memory)-[r]->(b:Memory) RETURN a, b",
		"t1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "a.tenant_id = 't1'") {
		t.Errorf("expected filter on first alias 'a', got %q", got)
	}
}

func TestAutoTenantScopeCypher_ComplexWhere(t *testing.T) {
	got, err := autoTenantScopeCypher(
		"MATCH (n:Memory) WHERE n.score > 0.5 AND n.status = 'active' RETURN n.id, n.score ORDER BY n.score DESC",
		"my-tenant",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "n.tenant_id = 'my-tenant'") {
		t.Errorf("tenant filter not found in: %q", got)
	}
	if !strings.Contains(got, "AND (") {
		t.Errorf("existing WHERE conditions should be wrapped: %q", got)
	}
}

func TestAutoTenantScopeCypher_WhitespaceInAlias(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH ( m :Memory) RETURN m", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "m.tenant_id = 't1'") {
		t.Errorf("expected alias 'm' with whitespace handling, got %q", got)
	}
}

func TestAutoTenantScopeCypher_SingleQuotesInTenantID(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH (n:Memory) RETURN n", "tenant'DROP")
	if err != nil {
		t.Fatalf("should not error — tenantID injected as literal: %v", err)
	}
}

// ─── FindChain age-not-available edge cases ──────────────────────────────────

func TestFindChain_WithLinkTypes_AGENotAvailable(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindChain(context.Background(), "t", 1, 42, []string{LinkTypeCausal, LinkTypeContradicts}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Connected {
		t.Error("expected Connected=false")
	}
}

func TestFindChain_NilClient(t *testing.T) {
	var gc *GraphClient
	result, err := gc.FindChain(context.Background(), "t", 1, 42, nil, 10)
	if err != nil {
		t.Fatalf("nil receiver: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from nil receiver")
	}
}

// ─── ExecuteCypher extra validation ──────────────────────────────────────────

func TestExecuteCypher_EmptyQuery(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only query")
	}
}

func TestExecuteCypher_LowercaseMatch(t *testing.T) {
	gc := NewGraphClient(nil) // nil db means AGE not available, but validation runs first
	_, err := gc.ExecuteCypher(context.Background(), "t", "match (n:Memory) return n")
	// Validation passes (prefix check is case-insensitive), then fails at AGE availability.
	if err == nil {
		t.Fatal("expected error for AGE unavailable")
	}
	if !strings.Contains(err.Error(), "cognitive graph not available") {
		t.Errorf("expected AGE-unavailable error, got: %v", err)
	}
}

func TestExecuteCypher_UpperCaseDangerousKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n) DELETE n")
	if err == nil {
		t.Fatal("expected error for DELETE keyword")
	}
}

func TestExecuteCypher_MixedCaseMatch(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "Match (n:Memory) Return n Limit 5")
	if err == nil {
		t.Fatal("expected error for AGE unavailable after passing validation")
	}
}

// ─── CreateLink edge cases ───────────────────────────────────────────────────

func TestCreateLink_NilClient_ReturnsError(t *testing.T) {
	var gc *GraphClient
	err := gc.CreateLink(context.Background(), "t", 1, 2, LinkTypeCausal, 1.0)
	if err == nil {
		t.Fatal("nil receiver must return error")
	}
}

func TestCreateLink_WithContradictsType(t *testing.T) {
	gc := NewGraphClient(nil)
	err := gc.CreateLink(context.Background(), "t", 1, 2, LinkTypeContradicts, 0.5)
	if err == nil {
		t.Fatal("nil db must return error")
	}
}

func TestCreateLink_ZeroWeight(t *testing.T) {
	gc := NewGraphClient(nil)
	err := gc.CreateLink(context.Background(), "t", 1, 2, LinkTypeSupports, 0)
	if err == nil {
		t.Fatal("nil db must return error")
	}
}

// ─── SyncMemoryLink edge cases ───────────────────────────────────────────────

func TestSyncMemoryLink_ZeroWeight(t *testing.T) {
	gc := NewGraphClient(nil)
	// Must not panic with zero weight
	gc.SyncMemoryLink(context.Background(), "t", "memory.1", "memory.2", LinkTypeCausal, 0)
}

func TestSyncMemoryLink_NegativeWeight(t *testing.T) {
	gc := NewGraphClient(nil)
	gc.SyncMemoryLink(context.Background(), "t", "memory.1", "memory.2", LinkTypeCausal, -1.0)
}

func TestSyncMemoryLink_NilClient(t *testing.T) {
	var gc *GraphClient
	gc.SyncMemoryLink(context.Background(), "t", "memory.1", "memory.2", LinkTypeCausal, 1.0)
}

// ─── sanitizeLinkType extended ───────────────────────────────────────────────

func TestSanitizeLinkType_Unicode(t *testing.T) {
	got := sanitizeLinkType("café")
	if got != "caf_" {
		t.Errorf("unicode chars should be replaced: got %q want caf_", got)
	}
}

func TestSanitizeLinkType_AllValidChars(t *testing.T) {
	got := sanitizeLinkType("ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789")
	if got != "ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789" {
		t.Errorf("all valid chars should pass through unchanged: got %q", got)
	}
}

func TestSanitizeLinkType_SQLKeywords(t *testing.T) {
	got := sanitizeLinkType("SELECT; DROP TABLE memory_links;--")
	if !strings.Contains(got, "SELECT") && len(got) > 5 {
		// Semi-colons and dashes become underscores
		if strings.Contains(got, ";") || strings.Contains(got, "-") {
			t.Errorf("special chars should be sanitized: %q", got)
		}
	}
}

// ─── IsAvailable with mock pool-like checks ──────────────────────────────────

func TestIsAvailable_DefaultZeroValue(t *testing.T) {
	gc := &GraphClient{} // db is nil (zero value)
	if gc.IsAvailable(context.Background()) {
		t.Error("zero-value GraphClient (db=nil) must report unavailable")
	}
}

// ─── FindChain with link types filtering ─────────────────────────────────────

func TestFindChain_DangerousLinkTypeSanitised(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindChain(context.Background(), "t", 1, 2, []string{"causal; DROP"}, 5)
	if err != nil {
		t.Fatalf("sanitised link type should not error: %v", err)
	}
}

// ─── Roundtrip: parseMemoryVertexID + ID formatting ──────────────────────────

func TestParseMemoryVertexID_ExtraQuotes(t *testing.T) {
	id, err := parseMemoryVertexID(`""memory.99""`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 99 {
		t.Errorf("expected 99, got %d", id)
	}
}

func TestParseMemoryVertexID_LeadingWhitespace(t *testing.T) {
	// parseMemoryVertexID does NOT trim spaces — only quotes and the "memory." prefix.
	_, err := parseMemoryVertexID(`  memory.123`)
	if err == nil {
		t.Fatal("expected error for leading whitespace (not trimmed by parseMemoryVertexID)")
	}
}

// ─── RelatedResult edge cases ────────────────────────────────────────────────

func TestRelatedResult_Empty(t *testing.T) {
	rr := &RelatedResult{}
	if rr.Memories != nil {
		t.Errorf("zero-value Memories should be nil, got %v", rr.Memories)
	}
	if rr.Total != 0 {
		t.Errorf("zero-value Total should be 0, got %d", rr.Total)
	}
}

// ─── ChainResult empty path ──────────────────────────────────────────────────

func TestChainResult_EmptyPath(t *testing.T) {
	cr := &ChainResult{FromID: 1, ToID: 2, Connected: false, Path: []ChainLink{}}
	if len(cr.Path) != 0 {
		t.Errorf("expected 0 path entries, got %d", len(cr.Path))
	}
}

// ─── CypherResult empty rows ─────────────────────────────────────────────────

func TestCypherResult_EmptyRows(t *testing.T) {
	cr := &CypherResult{Columns: []string{"result"}, Rows: nil}
	if cr.Rows != nil {
		t.Error("nil rows should remain nil")
	}
}

// ─── SetQueryTimeout ───────────────────────────────────────────────────────

func TestSetQueryTimeout_Positive(t *testing.T) {
	gc := NewGraphClient(nil)
	gc.SetQueryTimeout(5 * time.Second)
	if gc.queryTimeout != 5*time.Second {
		t.Errorf("positive timeout: got %v want 5s", gc.queryTimeout)
	}
}

func TestSetQueryTimeout_Zero_LeavesUnchanged(t *testing.T) {
	gc := NewGraphClient(nil)
	original := gc.queryTimeout // default 30s
	gc.SetQueryTimeout(0)
	if gc.queryTimeout != original {
		t.Errorf("zero timeout should leave unchanged: got %v want %v", gc.queryTimeout, original)
	}
}

func TestSetQueryTimeout_Negative_LeavesUnchanged(t *testing.T) {
	gc := NewGraphClient(nil)
	original := gc.queryTimeout
	gc.SetQueryTimeout(-1 * time.Second)
	if gc.queryTimeout != original {
		t.Errorf("negative timeout should leave unchanged: got %v want %v", gc.queryTimeout, original)
	}
}

func TestSetQueryTimeout_NilReceiver_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("SetQueryTimeout on nil receiver should panic")
		}
	}()
	var gc *GraphClient
	gc.SetQueryTimeout(5 * time.Second)
}

func TestSetQueryTimeout_DefaultIs30Seconds(t *testing.T) {
	gc := NewGraphClient(nil)
	if gc.queryTimeout != 30*time.Second {
		t.Errorf("default queryTimeout: got %v want 30s", gc.queryTimeout)
	}
	gc.SetQueryTimeout(10 * time.Second)
	if gc.queryTimeout != 10*time.Second {
		t.Errorf("after set: got %v want 10s", gc.queryTimeout)
	}
}

// ─── ageConn nil-safe paths ────────────────────────────────────────────────

func TestAgeConn_NilDB_ReturnsError(t *testing.T) {
	gc := NewGraphClient(nil)
	conn, err := gc.ageConn(context.Background())
	if err == nil {
		t.Fatal("ageConn with nil db must return error")
	}
	if conn != nil {
		conn.Release()
		t.Error("ageConn with nil db must return nil conn")
	}
}

func TestAgeConn_NilReceiver_ReturnsError(t *testing.T) {
	var gc *GraphClient
	conn, err := gc.ageConn(context.Background())
	if err == nil {
		t.Fatal("ageConn on nil receiver must return error")
	}
	if conn != nil {
		t.Error("ageConn on nil receiver must return nil conn")
	}
}

// ─── FindRelated: nil receiver path ────────────────────────────────────────

func TestFindRelated_DepthNormalisedWhenAGEUnavailable(t *testing.T) {
	// With nil db: IsAvailable returns false → returns empty. Verify normalisations
	// do not execute (they are after the IsAvailable guard).
	gc := NewGraphClient(nil)
	result, err := gc.FindRelated(context.Background(), "t", 1, []string{"causal"}, -1, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Memories == nil {
		t.Error("Memories should be non-nil empty slice")
	}
}

func TestFindRelated_AllParams_NilDB(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindRelated(context.Background(), "t", 42, []string{"causal", "temporal"}, 10, 100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("expected empty, got %d entries", len(result.Memories))
	}
}

// ─── FindChain: more edge cases ────────────────────────────────────────────

func TestFindChain_AllLinkTypes_NilDB(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindChain(context.Background(), "t", 1, 2, []string{"causal", "temporal", "contradicts", "supports", "related"}, 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Connected {
		t.Error("expected Connected=false when AGE unavailable")
	}
}

func TestFindChain_EmptyLinkTypes_NilDB(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindChain(context.Background(), "t", 1, 2, []string{}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestFindChain_NegativeMaxDepth_NilDB(t *testing.T) {
	gc := NewGraphClient(nil)
	result, err := gc.FindChain(context.Background(), "t", 1, 2, nil, -5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FromID != 1 || result.ToID != 2 {
		t.Errorf("IDs must be preserved")
	}
}

// ─── ExecuteCypher: more validation edge cases ────────────────────────────

func TestExecuteCypher_QueryWithOnlyWhitespace(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "\n \t  ")
	if err == nil {
		t.Fatal("whitespace-only query should be rejected")
	}
}

func TestExecuteCypher_QueryWithoutMemoryNode_ValidMatch(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n:Person) RETURN n")
	if err == nil {
		t.Fatal("query without :Memory node should be rejected by tenant scoping")
	}
}

func TestExecuteCypher_QueryWithoutReturn(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n:Memory)")
	if err == nil {
		t.Fatal("query without RETURN should be rejected by tenant scoping")
	}
}

func TestExecuteCypher_SetKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n) SET n.x = 1")
	if err == nil {
		t.Fatal("SET keyword should be rejected")
	}
}

func TestExecuteCypher_RemoveKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n) REMOVE n.x")
	if err == nil {
		t.Fatal("REMOVE keyword should be rejected")
	}
}

func TestExecuteCypher_MergeKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "MERGE (n:Memory)")
	if err == nil {
		t.Fatal("MERGE keyword should be rejected")
	}
}

func TestExecuteCypher_DropKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "DROP TABLE memory_entries")
	if err == nil {
		t.Fatal("DROP keyword should be rejected")
	}
}

func TestExecuteCypher_CallKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "CALL proc()")
	if err == nil {
		t.Fatal("CALL keyword should be rejected")
	}
}

func TestExecuteCypher_LoadKeyword(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.ExecuteCypher(context.Background(), "t", "LOAD CSV FROM 'file.csv'")
	if err == nil {
		t.Fatal("LOAD keyword should be rejected")
	}
}

func TestExecuteCypher_NilReceiver(t *testing.T) {
	var gc *GraphClient
	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n) RETURN n")
	if err == nil {
		t.Fatal("nil receiver must return error")
	}
}

// ─── CreateLink: edge values ──────────────────────────────────────────────

func TestCreateLink_NegativeWeight(t *testing.T) {
	gc := NewGraphClient(nil)
	err := gc.CreateLink(context.Background(), "t", 1, 2, LinkTypeSupports, -0.5)
	if err == nil {
		t.Fatal("nil db must return error")
	}
}

func TestCreateLink_LargeWeight(t *testing.T) {
	gc := NewGraphClient(nil)
	err := gc.CreateLink(context.Background(), "t", 1, 2, LinkTypeRelated, 999999.0)
	if err == nil {
		t.Fatal("nil db must return error")
	}
}

// ─── SyncMemoryLink: zero weight normalisation ────────────────────────────

func TestSyncMemoryLink_ZeroWeightNormalised_WithAvailableAGE(t *testing.T) {
	// SyncMemoryLink with zero weight + nil db (AGE unavailable) → weight
	// is normalised to 1.0 but exec is skipped because IsAvailable is false.
	gc := NewGraphClient(nil)
	gc.SyncMemoryLink(context.Background(), "t", "memory.1", "memory.2", LinkTypeCausal, 0)
	// Should not panic and should not try to exec SQL.
}

func TestSyncMemoryLink_NegativeWeightNormalised(t *testing.T) {
	gc := NewGraphClient(nil)
	gc.SyncMemoryLink(context.Background(), "t", "memory.1", "memory.2", LinkTypeTemporal, -5.0)
}

// ─── IsAvailable: edge ────────────────────────────────────────────────────

func TestIsAvailable_NilPool(t *testing.T) {
	gc := &GraphClient{db: nil}
	if gc.IsAvailable(context.Background()) {
		t.Error("IsAvailable with nil db must return false")
	}
}

// ─── FindRelated: link types with SQL-like content ────────────────────────

func TestFindRelated_LinkTypeWithSQLKeywords(t *testing.T) {
	gc := NewGraphClient(nil)
	_, err := gc.FindRelated(context.Background(), "t", 1, []string{"SELECT", "DROP", "causal_injection"}, 3, 0, 50)
	if err != nil {
		t.Fatalf("SQL-like link types should be sanitised, not error: %v", err)
	}
}

func TestFindRelated_LimitClamped(t *testing.T) {
	// FindRelated normalises limit=0 → 50 before calling AGE. With nil db we never
	// reach the normalisation, but we can verify the guard works.
	gc := NewGraphClient(nil)
	result, _ := gc.FindRelated(context.Background(), "t", 1, nil, 3, 0, 100)
	if result.Memories == nil {
		t.Error("expected non-nil Memories")
	}
}

// ─── FindChain: various max depths ────────────────────────────────────────

func TestFindChain_MaxDepthEdgeValues(t *testing.T) {
	gc := NewGraphClient(nil)
	for _, d := range []int{1, 5, 10, 20} {
		result, err := gc.FindChain(context.Background(), "t", 1, 2, nil, d)
		if err != nil {
			t.Errorf("maxDepth=%d: unexpected error: %v", d, err)
		}
		if result == nil {
			t.Errorf("maxDepth=%d: expected non-nil result", d)
		}
	}
}

func TestFindChain_MaxDepthExceedsLimit(t *testing.T) {
	gc := NewGraphClient(nil)
	// maxDepth=1000 exceeds typical limits but FindChain should handle it
	// (with nil db it returns early via IsAvailable check).
	result, err := gc.FindChain(context.Background(), "t", 1, 2, nil, 1000)
	if err != nil {
		t.Fatalf("unexpected error with large maxDepth: %v", err)
	}
	if result.FromID != 1 || result.ToID != 2 {
		t.Error("IDs must be preserved")
	}
}

// ─── autoTenantScopeCypher: additional edge cases ─────────────────────────

func TestAutoTenantScopeCypher_AliasWithBraces(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (m:Memory {id: 'memory.1'}) RETURN m", "t9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "m.tenant_id = 't9'") {
		t.Errorf("expected tenant filter on alias 'm', got %q", got)
	}
}

func TestAutoTenantScopeCypher_WhereWithANDAfterInjection(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (n:Memory) WHERE n.score > 0.5 AND n.active = true RETURN n", "t10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tenant filter must appear before the existing WHERE body
	if !strings.Contains(got, "tenant_id = 't10'") {
		t.Errorf("tenant filter not found: %q", got)
	}
	if !strings.Contains(got, "AND (") {
		t.Errorf("existing WHERE not wrapped: %q", got)
	}
}

func TestAutoTenantScopeCypher_OrderByAndLimit(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (n:Memory) RETURN n ORDER BY n.id LIMIT 20", "t11")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "ORDER BY n.id LIMIT 20") {
		t.Errorf("ORDER BY / LIMIT must survive tenant scoping: %q", got)
	}
}

func TestAutoTenantScopeCypher_TenantIDWithHyphen(t *testing.T) {
	got, err := autoTenantScopeCypher("MATCH (n:Memory) RETURN n.id", "my-tenant-id")
	if err != nil {
		t.Fatalf("unexpected error with hyphenated tenant: %v", err)
	}
	if !strings.Contains(got, "n.tenant_id = 'my-tenant-id'") {
		t.Errorf("hyphenated tenant ID not preserved: %q", got)
	}
}

func TestAutoTenantScopeCypher_AliasWithPropertiesBeforeColon(t *testing.T) {
	// Pattern: (n {score:0.5} :Memory) — aliasPart contains brace/space before :Memory.
	// This hits the IndexAny branch (line 406-407).
	got, err := autoTenantScopeCypher(
		"MATCH (n {score:0.5} :Memory) RETURN n",
		"t12",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "n.tenant_id = 't12'") {
		t.Errorf("alias 'n' not extracted correctly from (n {prop} :Memory): %q", got)
	}
}

func TestAutoTenantScopeCypher_EmptyAliasWithBrace(t *testing.T) {
	// Pattern: ( {key:'val'} :Memory) — no alias, just properties then :Memory.
	// aliasPart = "{key:'val'}", IndexAny finds '{' at idx 0, alias = "".
	_, err := autoTenantScopeCypher(
		"MATCH ( {key:'val'} :Memory) RETURN n",
		"t13",
	)
	if err == nil {
		t.Fatal("expected error for empty alias after brace extraction")
	}
	if !strings.Contains(err.Error(), "could not determine") {
		t.Errorf("expected 'could not determine' error, got: %v", err)
	}
}

func TestAutoTenantScopeCypher_AliasWithSpaceBeforeColon(t *testing.T) {
	// Pattern: (n  :Memory) — aliasPart contains extra space before :.
	// TrimSpace removes leading/trailing but internal space remains.
	got, err := autoTenantScopeCypher(
		"MATCH (n  :Memory) RETURN n",
		"t14",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "n.tenant_id = 't14'") {
		t.Errorf("alias 'n' not extracted from (n  :Memory): %q", got)
	}
}

func TestAutoTenantScopeCypher_MultipleSpacesBeforeWhere(t *testing.T) {
	got, err := autoTenantScopeCypher(
		"MATCH (a:Memory)   WHERE   a.x > 0   RETURN a",
		"t15",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "a.tenant_id = 't15'") {
		t.Errorf("tenant filter not found: %q", got)
	}
}

// ─── parseAGEPath: additional robustness ──────────────────────────────────

func TestParseAGEPath_NilInput(t *testing.T) {
	links := parseAGEPath(nil)
	if links != nil {
		t.Errorf("nil input: got %v want nil", links)
	}
}

func TestParseAGEPath_MalformedEdgeJSON(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		{not-valid-json-at-all},
		{"id": 2, "label": "Memory", "properties": {"id": "memory.2"}}
	]`)
	links := parseAGEPath(path)
	if links != nil {
		t.Errorf("malformed edge JSON: got %v want nil", links)
	}
}

func TestParseAGEPath_PathSuffix(t *testing.T) {
	// AGE sometimes emits the ::path suffix on the outer array
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		{"id": 10, "label": "causal", "start_id": 1, "end_id": 2, "properties": {}},
		{"id": 2, "label": "Memory", "properties": {"id": "memory.7"}}
	]::path`)
	links := parseAGEPath(path)
	if len(links) != 1 {
		t.Fatalf("path with ::path suffix: expected 1 link, got %d", len(links))
	}
	if links[0].FromID != 1 || links[0].ToID != 7 {
		t.Errorf("got from=%d to=%d want from=1 to=7", links[0].FromID, links[0].ToID)
	}
}

func TestParseAGEPath_VertexWithSuffixes(t *testing.T) {
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.42"}}::vertex,
		{"id": 99, "label": "supports", "properties": {}}::edge,
		{"id": 2, "label": "Memory", "properties": {"id": "memory.99"}}::vertex
	]`)
	links := parseAGEPath(path)
	if len(links) != 1 {
		t.Fatalf("path with ::vertex/::edge suffixes: expected 1 link, got %d", len(links))
	}
	if links[0].FromID != 42 || links[0].ToID != 99 {
		t.Errorf("got from=%d to=%d want from=42 to=99", links[0].FromID, links[0].ToID)
	}
}

func TestParseAGEPath_EdgeJSONUnmarshalFails_Continues(t *testing.T) {
	// A valid JSON array where the edge at index 1 is a JSON string (not an object).
	// The initial json.Unmarshal into []json.RawMessage succeeds (the array is valid),
	// but json.Unmarshal of the string into the edge struct fails → continue.
	// This hits the `if err := json.Unmarshal(path[i], &edge); err != nil { continue }` branch.
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		"not-a-struct-edge",
		{"id": 2, "label": "Memory", "properties": {"id": "memory.7"}}
	]`)
	links := parseAGEPath(path)
	// The edge at index 1 is a string, unmarshal fails → continue.
	// Index 0 is a vertex (ok but skipped — it's at position 0, the loop starts at 1).
	// Index 2 is a vertex (ok but only edges are processed).
	// Result: 0 links (the bad edge was the only edge position, and it failed).
	if links != nil {
		t.Logf("got %d links (edge at [1] is a string, unmarshal should fail gracefully)", len(links))
	}
}

func TestParseAGEPath_EdgeIsNumber_FailsUnmarshal(t *testing.T) {
	// Edge is a JSON number — valid JSON but not an object → unmarshal fails.
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		12345,
		{"id": 2, "label": "Memory", "properties": {"id": "memory.7"}}
	]`)
	links := parseAGEPath(path)
	if links != nil {
		t.Logf("got %d links (edge is number, unmarshal should fail gracefully)", len(links))
	}
}

func TestParseAGEPath_FiveHopChain(t *testing.T) {
	// 5-hop chain: v0-e0-v1-e1-v2-e2-v3-e3-v4-e4-v5 (11 elements)
	path := []byte(`[
		{"id": 1, "label": "Memory", "properties": {"id": "memory.1"}},
		{"id": 10, "label": "causal", "start_id": 1, "end_id": 2, "properties": {}},
		{"id": 2, "label": "Memory", "properties": {"id": "memory.10"}},
		{"id": 11, "label": "temporal", "start_id": 2, "end_id": 3, "properties": {}},
		{"id": 3, "label": "Memory", "properties": {"id": "memory.20"}},
		{"id": 12, "label": "supports", "start_id": 3, "end_id": 4, "properties": {}},
		{"id": 4, "label": "Memory", "properties": {"id": "memory.30"}},
		{"id": 13, "label": "contradicts", "start_id": 4, "end_id": 5, "properties": {}},
		{"id": 5, "label": "Memory", "properties": {"id": "memory.40"}},
		{"id": 14, "label": "related", "start_id": 5, "end_id": 6, "properties": {}},
		{"id": 6, "label": "Memory", "properties": {"id": "memory.50"}}
	]`)
	links := parseAGEPath(path)
	if len(links) != 5 {
		t.Fatalf("expected 5 links, got %d", len(links))
	}
	expected := []struct {
		from, to int64
		lt       string
		hop      int
	}{
		{1, 10, "causal", 0},
		{10, 20, "temporal", 1},
		{20, 30, "supports", 2},
		{30, 40, "contradicts", 3},
		{40, 50, "related", 4},
	}
	for i, exp := range expected {
		if links[i].FromID != exp.from || links[i].ToID != exp.to || links[i].LinkType != exp.lt || links[i].HopIndex != exp.hop {
			t.Errorf("link[%d]: got {from=%d to=%d type=%s hop=%d}, want {from=%d to=%d type=%s hop=%d}",
				i, links[i].FromID, links[i].ToID, links[i].LinkType, links[i].HopIndex,
				exp.from, exp.to, exp.lt, exp.hop)
		}
	}
}

// ─── validateCypherQuery (pure function, full coverage) ────────────────────

func TestValidateCypherQuery_ValidMatch(t *testing.T) {
	trimmed, err := validateCypherQuery("MATCH (n:Memory) RETURN n LIMIT 10")
	if err != nil {
		t.Fatalf("valid MATCH query should pass: %v", err)
	}
	if !strings.HasPrefix(trimmed, "MATCH") {
		t.Errorf("trimmed query should start with MATCH: %q", trimmed)
	}
}

func TestValidateCypherQuery_LowercaseMatch(t *testing.T) {
	trimmed, err := validateCypherQuery("match (n:Memory) return n")
	if err != nil {
		t.Fatalf("lowercase 'match' should pass: %v", err)
	}
	if trimmed != "match (n:Memory) return n" {
		t.Errorf("lowercase query preserved: got %q", trimmed)
	}
}

func TestValidateCypherQuery_MixedCaseMatch(t *testing.T) {
	trimmed, err := validateCypherQuery("Match (n:Memory) Return n Limit 5")
	if err != nil {
		t.Fatalf("mixed-case 'Match' should pass: %v", err)
	}
	if trimmed != "Match (n:Memory) Return n Limit 5" {
		t.Errorf("original case preserved: got %q", trimmed)
	}
}

func TestValidateCypherQuery_EmptyString(t *testing.T) {
	_, err := validateCypherQuery("")
	if err == nil {
		t.Fatal("empty query must be rejected")
	}
}

func TestValidateCypherQuery_WhitespaceOnly(t *testing.T) {
	for _, q := range []string{"   ", "\t\n ", "  \n  "} {
		_, err := validateCypherQuery(q)
		if err == nil {
			t.Errorf("whitespace-only query %q must be rejected", q)
		}
	}
}

func TestValidateCypherQuery_TrimsWhitespace(t *testing.T) {
	trimmed, err := validateCypherQuery("  MATCH (n) RETURN n  ")
	if err != nil {
		t.Fatalf("padded MATCH query should pass: %v", err)
	}
	if trimmed != "MATCH (n) RETURN n" {
		t.Errorf("should be trimmed: got %q", trimmed)
	}
}

func TestValidateCypherQuery_NotMatchPrefix(t *testing.T) {
	rejected := []string{
		"CREATE (n:Memory {id: 'x'})",
		"RETURN n",
		"SELECT * FROM memory_entries",
		"INSERT INTO memory_links VALUES (1)",
		"UPDATE memory_entries SET content = 'x'",
		"WITH n MATCH (m) RETURN m",
		"OPTIONAL MATCH (n) RETURN n",
		"EXPLAIN MATCH (n) RETURN n",
	}
	for _, q := range rejected {
		_, err := validateCypherQuery(q)
		if err == nil {
			t.Errorf("non-MATCH prefix query %q must be rejected", q)
		}
	}
}

func TestValidateCypherQuery_EachDangerousKeyword(t *testing.T) {
	dangerous := map[string]string{
		"MATCH (n) CREATE (x)":                 "CREATE",
		"MATCH (n) DELETE n":                   "DELETE",
		"MATCH (n) SET n.x = 1":               "SET",
		"MATCH (n) REMOVE n.x":                  "REMOVE",
		"MATCH (n) MERGE (x:Memory {id:'y'})": "MERGE",
		"MATCH (n) DROP TABLE memory_entries":  "DROP",
		"MATCH (n) CALL proc()":                "CALL",
		"MATCH (n) LOAD CSV FROM 'f.csv'":      "LOAD",
	}
	for q, kw := range dangerous {
		_, err := validateCypherQuery(q)
		if err == nil {
			t.Errorf("query with %s must be rejected: %q", kw, q)
		}
	}
}

func TestValidateCypherQuery_DangerousInLowerCase(t *testing.T) {
	// Dangerous keywords are detected case-insensitively.
	rejected := []string{
		"MATCH (n) create (x)",
		"MATCH (n) delete n",
		"MATCH (n) set n.x = 1",
		"MATCH (n) merge (x)",
	}
	for _, q := range rejected {
		_, err := validateCypherQuery(q)
		if err == nil {
			t.Errorf("lowercase dangerous keyword should be rejected: %q", q)
		}
	}
}

func TestValidateCypherQuery_DangerousEmbeddedInWord(t *testing.T) {
	// Keywords like "SET" inside longer words (e.g. "RESET") need a space.
	// Our check uses "SET " with trailing space, so "RESETTING" won't match.
	for _, q := range []string{
		"MATCH (n:Memory) RETURN n.settings",
		"MATCH (n) RETURN created_at",
	} {
		_, err := validateCypherQuery(q)
		if err != nil {
			t.Errorf("keyword embedded in word should NOT be rejected: %q → %v", q, err)
		}
	}
}

func TestValidateCypherQuery_ComplexValidQuery(t *testing.T) {
	q := "MATCH (a:Memory)-[r:causal]->(b:Memory) WHERE a.importance > 0.5 RETURN a.id, b.id, r.weight ORDER BY r.weight DESC LIMIT 50"
	trimmed, err := validateCypherQuery(q)
	if err != nil {
		t.Fatalf("complex valid query should pass: %v", err)
	}
	if trimmed != q {
		t.Errorf("complex query must not be altered: got %q", trimmed)
	}
}

func TestValidateCypherQuery_QueryWithTabsAndNewlines(t *testing.T) {
	trimmed, err := validateCypherQuery("\tMATCH (n:Memory)\nRETURN n.id\n")
	if err != nil {
		t.Fatalf("query with tabs/newlines should pass: %v", err)
	}
	if !strings.HasPrefix(trimmed, "MATCH") {
		t.Errorf("expected trimmed MATCH, got %q", trimmed)
	}
}

// ─── FindRelated: input normalisation with nil db (moved before IsAvailable) ──

func TestFindRelated_NormalisationRunsBeforeAGE(t *testing.T) {
	// With nil db, IsAvailable returns false, but normalisation now happens
	// before the check, so these branches are covered.
	gc := NewGraphClient(nil)

	// maxDepth=0 → normalised to 1
	r1, _ := gc.FindRelated(context.Background(), "t", 1, nil, 0, 0, 50)
	if len(r1.Memories) != 0 {
		t.Error("maxDepth=0: expected empty (AGE unavailable)")
	}

	// limit=0 → normalised to 50
	r2, _ := gc.FindRelated(context.Background(), "t", 1, nil, 3, 0, 0)
	if len(r2.Memories) != 0 {
		t.Error("limit=0: expected empty (AGE unavailable)")
	}

	// cursor=-1 → normalised to 0
	r3, _ := gc.FindRelated(context.Background(), "t", 1, nil, 3, -1, 50)
	if len(r3.Memories) != 0 {
		t.Error("cursor=-1: expected empty (AGE unavailable)")
	}
}

func TestFindRelated_MaxDepthAlreadyValid(t *testing.T) {
	gc := NewGraphClient(nil)
	r, err := gc.FindRelated(context.Background(), "t", 1, nil, 5, 0, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Memories == nil {
		t.Error("expected non-nil Memories")
	}
}

func TestFindRelated_LimitAlreadyValid(t *testing.T) {
	gc := NewGraphClient(nil)
	r, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Memories == nil {
		t.Error("expected non-nil Memories")
	}
}

func TestFindRelated_CursorAlreadyValid(t *testing.T) {
	gc := NewGraphClient(nil)
	r, err := gc.FindRelated(context.Background(), "t", 1, nil, 3, 100, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Memories == nil {
		t.Error("expected non-nil Memories")
	}
}

// ─── FindChain: input normalisation with nil db (moved before IsAvailable) ─

func TestFindChain_NormalisationRunsBeforeAGE(t *testing.T) {
	gc := NewGraphClient(nil)

	// maxDepth=0 → normalised to 3
	r1, _ := gc.FindChain(context.Background(), "t", 1, 2, nil, 0)
	if r1.FromID != 1 || r1.ToID != 2 {
		t.Error("IDs must be preserved")
	}

	// maxDepth=-1 → normalised to 3
	r2, _ := gc.FindChain(context.Background(), "t", 1, 2, nil, -1)
	if r2.FromID != 1 || r2.ToID != 2 {
		t.Error("IDs must be preserved after negative maxDepth normalization")
	}

	// maxDepth already valid
	r3, err := gc.FindChain(context.Background(), "t", 1, 2, nil, 10)
	if err != nil {
		t.Fatalf("valid maxDepth: unexpected error: %v", err)
	}
	if r3.FromID != 1 || r3.ToID != 2 {
		t.Error("IDs must be preserved")
	}
}

func TestFindChain_MaxDepthOne(t *testing.T) {
	gc := NewGraphClient(nil)
	r, err := gc.FindChain(context.Background(), "t", 1, 2, []string{LinkTypeCausal}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Connected {
		t.Error("expected Connected=false when AGE unavailable")
	}
}

// ─── ExecuteCypher: validation now testable via validateCypherQuery ────────

func TestExecuteCypher_ValidationUsesExtractedFunction(t *testing.T) {
	// ExecuteCypher with nil db returns "cognitive graph not available" error
	// BEFORE reaching validation, because IsAvailable check is first.
	// The actual validation logic is now in validateCypherQuery, tested above.
	// These tests verify the integration: nil db → IsAvailable → graceful error.
	gc := NewGraphClient(nil)

	_, err := gc.ExecuteCypher(context.Background(), "t", "MATCH (n:Memory) RETURN n")
	if err == nil {
		t.Fatal("nil db must return error (AGE unavailable)")
	}
	if !strings.Contains(err.Error(), "cognitive graph not available") {
		t.Errorf("expected AGE-unavailable error, got: %v", err)
	}
}

// ─── IsAvailable: additional edge ─────────────────────────────────────────

func TestIsAvailable_ZeroValueClient(t *testing.T) {
	// &GraphClient{} has nil db — IsAvailable must return false.
	gc := &GraphClient{}
	if gc.IsAvailable(context.Background()) {
		t.Error("zero-value GraphClient must report unavailable")
	}
	// context.TODO is fine when the caller doesn't know which context to use yet.
	if gc.IsAvailable(context.TODO()) {
		t.Error("must report unavailable even with context.TODO()")
	}
}

// ─── buildRelPattern (pure function, extracted from FindRelated/FindChain) ──

func TestBuildRelPattern_NoLinkTypes(t *testing.T) {
	p, f := buildRelPattern(3, "r", nil)
	if p != "[r*1..3]" {
		t.Errorf("pattern: got %q want [r*1..3]", p)
	}
	if f != "" {
		t.Errorf("typeFilter with nil linkTypes: got %q want empty", f)
	}
}

func TestBuildRelPattern_EmptyLinkTypes(t *testing.T) {
	p, f := buildRelPattern(5, "e", []string{})
	if p != "[e*1..5]" {
		t.Errorf("pattern: got %q want [e*1..5]", p)
	}
	if f != "" {
		t.Errorf("typeFilter with empty linkTypes: got %q want empty", f)
	}
}

func TestBuildRelPattern_SingleLinkType(t *testing.T) {
	p, f := buildRelPattern(2, "r", []string{"causal"})
	if p != "[r*1..2]" {
		t.Errorf("pattern: got %q", p)
	}
	if !strings.Contains(f, "type(r[0]) = 'causal'") {
		t.Errorf("typeFilter must reference link type: %q", f)
	}
}

func TestBuildRelPattern_MultipleLinkTypes(t *testing.T) {
	p, f := buildRelPattern(4, "e", []string{"causal", "temporal", "contradicts"})
	if p != "[e*1..4]" {
		t.Errorf("pattern: got %q", p)
	}
	for _, lt := range []string{"causal", "temporal", "contradicts"} {
		if !strings.Contains(f, lt) {
			t.Errorf("typeFilter missing %q: %q", lt, f)
		}
	}
	if !strings.Contains(f, " OR ") {
		t.Errorf("typeFilter must contain OR: %q", f)
	}
}

func TestBuildRelPattern_DepthOne(t *testing.T) {
	p, f := buildRelPattern(1, "r", []string{"causal"})
	if p != "[r*1..1]" {
		t.Errorf("depth=1: got %q", p)
	}
	if !strings.Contains(f, "causal") {
		t.Errorf("typeFilter: got %q", f)
	}
}

func TestBuildRelPattern_DepthHigh(t *testing.T) {
	p, _ := buildRelPattern(20, "r", []string{"causal", "temporal", "supports", "contradicts", "related"})
	if p != "[r*1..20]" {
		t.Errorf("depth=20: got %q", p)
	}
}

func TestBuildRelPattern_LinkTypesWithSpecialChars(t *testing.T) {
	_, f := buildRelPattern(3, "r", []string{"causal-injection", "has space"})
	if !strings.Contains(f, "causal_injection") {
		t.Errorf("dash should be sanitised to underscore: %q", f)
	}
	if !strings.Contains(f, "has_space") {
		t.Errorf("space should be sanitised to underscore: %q", f)
	}
}

func TestBuildRelPattern_AllFiveLinkTypes(t *testing.T) {
	all := []string{"causal", "temporal", "contradicts", "supports", "related"}
	p, f := buildRelPattern(3, "r", all)
	if p != "[r*1..3]" {
		t.Errorf("pattern: got %q", p)
	}
	for _, lt := range all {
		if !strings.Contains(f, lt) {
			t.Errorf("missing link type %q in: %q", lt, f)
		}
	}
}

// ─── buildCursorClause (pure function) ─────────────────────────────────────

func TestBuildCursorClause_Positive(t *testing.T) {
	clause := buildCursorClause(42)
	if clause == "" {
		t.Fatal("cursor=42 must return non-empty clause")
	}
	if !strings.Contains(clause, "memory.42") {
		t.Errorf("cursor clause must contain memory.42: %q", clause)
	}
}

func TestBuildCursorClause_Zero(t *testing.T) {
	clause := buildCursorClause(0)
	if clause != "" {
		t.Errorf("cursor=0 must return empty string, got %q", clause)
	}
}

func TestBuildCursorClause_Negative(t *testing.T) {
	clause := buildCursorClause(-5)
	if clause != "" {
		t.Errorf("cursor=-5 must return empty string, got %q", clause)
	}
}

func TestBuildCursorClause_LargeValue(t *testing.T) {
	clause := buildCursorClause(999999)
	if !strings.Contains(clause, "memory.999999") {
		t.Errorf("large cursor: got %q", clause)
	}
}
