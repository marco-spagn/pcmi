package graph

import (
	"testing"
	"time"
)

func TestNewGraphClient(t *testing.T) {
	g := NewGraphClient(nil)
	if g == nil {
		t.Fatal("NewGraphClient returned nil")
	}
	if g.queryTimeout != 30*time.Second {
		t.Errorf("expected 30s default timeout, got %v", g.queryTimeout)
	}
}

func TestSetQueryTimeout(t *testing.T) {
	g := &GraphClient{queryTimeout: 30 * time.Second}
	g.SetQueryTimeout(10 * time.Second)
	if g.queryTimeout != 10*time.Second {
		t.Errorf("expected 10s, got %v", g.queryTimeout)
	}
}

func TestSetQueryTimeout_zero(t *testing.T) {
	g := &GraphClient{queryTimeout: 30 * time.Second}
	g.SetQueryTimeout(0)
	if g.queryTimeout != 30*time.Second {
		t.Errorf("expected unchanged (30s), got %v", g.queryTimeout)
	}
}

func TestSetQueryTimeout_negative(t *testing.T) {
	g := &GraphClient{queryTimeout: 30 * time.Second}
	g.SetQueryTimeout(-1 * time.Second)
	if g.queryTimeout != 30*time.Second {
		t.Errorf("expected unchanged (30s), got %v", g.queryTimeout)
	}
}

func TestIsAvailable_nilGraph(t *testing.T) {
	var g *GraphClient
	if g.IsAvailable(t.Context()) {
		t.Error("nil graph should not be available")
	}
}

func TestIsAvailable_nilDB(t *testing.T) {
	g := &GraphClient{db: nil}
	if g.IsAvailable(t.Context()) {
		t.Error("nil db should not be available")
	}
}

func TestAgeConn_nilGraph(t *testing.T) {
	var g *GraphClient
	_, err := g.ageConn(t.Context())
	if err == nil {
		t.Error("expected error for nil graph")
	}
}

func TestAgeConn_nilDB(t *testing.T) {
	g := &GraphClient{db: nil}
	_, err := g.ageConn(t.Context())
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestCreateLink_nilGraph(t *testing.T) {
	var g *GraphClient
	err := g.CreateLink(t.Context(), "t1", 1, 2, "related", 1.0)
	if err == nil {
		t.Error("expected error for nil graph")
	}
}

func TestCreateLink_nilDB(t *testing.T) {
	g := &GraphClient{db: nil}
	err := g.CreateLink(t.Context(), "t1", 1, 2, "related", 1.0)
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestValidateCypherQuery_valid(t *testing.T) {
	tests := []string{
		"MATCH (n:Memory) RETURN n",
		"match (n:Memory) return n",
		"  MATCH (n:Memory {id: '123'}) RETURN n  ",
		"MATCH (a:Memory)-[r:RELATED]->(b:Memory) RETURN a, r, b",
	}
	for _, q := range tests {
		got, err := validateCypherQuery(q)
		if err != nil {
			t.Errorf("validateCypherQuery(%q) unexpected error: %v", q, err)
			continue
		}
		if got == "" {
			t.Errorf("validateCypherQuery(%q) returned empty string", q)
		}
	}
}

func TestValidateCypherQuery_invalid(t *testing.T) {
	tests := []struct {
		query string
		why   string
	}{
		{"", "empty"},
		{"  ", "whitespace only"},
		{"RETURN n", "not MATCH"},
		{"CREATE (n:Memory) RETURN n", "write keyword CREATE"},
		{"MATCH (n:Memory) DELETE n", "write keyword DELETE"},
		{"MATCH (n:Memory) SET n.x = 1 RETURN n", "write keyword SET"},
		{"MATCH (n:Memory) REMOVE n.x RETURN n", "write keyword REMOVE"},
		{"MATCH (n:Memory) MERGE (n)-[:R]->(m) RETURN n", "write keyword MERGE"},
		{"MATCH (n:Memory) CALL foo() RETURN n", "write keyword CALL"},
		{"MATCH (n:Memory) RETURN n LOAD CSV FROM 'x'", "write keyword LOAD"},
	}
	for _, tt := range tests {
		_, err := validateCypherQuery(tt.query)
		if err == nil {
			t.Errorf("validateCypherQuery(%q) expected error (%s), got nil", tt.query, tt.why)
		}
	}
}

func TestAutoTenantScopeCypher(t *testing.T) {
	query := "MATCH (n:Memory) RETURN n"
	got, err := autoTenantScopeCypher(query, "t-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == query {
		t.Error("query was not modified")
	}
	// Should contain the tenant filter
	if !contains(got, "t-123") {
		t.Errorf("expected tenant filter, got: %s", got)
	}
}

func TestAutoTenantScopeCypher_noMemoryRef(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH (n:Node) RETURN n", "t-123")
	if err == nil {
		t.Error("expected error for query without :Memory reference")
	}
}

func TestAutoTenantScopeCypher_noReturn(t *testing.T) {
	_, err := autoTenantScopeCypher("MATCH (n:Memory)", "t-123")
	if err == nil {
		t.Error("expected error for query without RETURN")
	}
}

func TestBuildRelPattern_emptyTypes(t *testing.T) {
	pattern, typeFilter := buildRelPattern(3, "r", nil)
	if pattern != "[r*1..3]" {
		t.Errorf("unexpected pattern: %s", pattern)
	}
	if typeFilter != "" {
		t.Errorf("expected empty type filter, got: %s", typeFilter)
	}
}

func TestBuildRelPattern_withTypes(t *testing.T) {
	pattern, typeFilter := buildRelPattern(2, "e", []string{"related", "derived_from"})
	if pattern != "[e*1..2]" {
		t.Errorf("unexpected pattern: %s", pattern)
	}
	if typeFilter != "" {
		t.Errorf("expected empty type filter when filtering happens after AGE returns paths, got: %s", typeFilter)
	}
}

func TestBuildCursorClause_zero(t *testing.T) {
	if got := buildCursorClause(0); got != "" {
		t.Errorf("expected empty for cursor=0, got: %s", got)
	}
}

func TestBuildCursorClause_negative(t *testing.T) {
	if got := buildCursorClause(-1); got != "" {
		t.Errorf("expected empty for cursor=-1, got: %s", got)
	}
}

func TestBuildCursorClause_positive(t *testing.T) {
	got := buildCursorClause(42)
	if got == "" {
		t.Error("expected non-empty clause for cursor=42")
	}
	if !contains(got, "memory.42") {
		t.Errorf("expected cursor to reference memory.42, got: %s", got)
	}
}

func TestParseAGEPath_valid(t *testing.T) {
	raw := []byte(`[{"id":"memory.1"}::vertex, {"label":"related","start_id":1,"end_id":2,"properties":{}}::edge, {"id":"memory.2"}::vertex]::path`)
	links := parseAGEPath(raw)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].LinkType != "related" {
		t.Errorf("expected link type 'related', got %q", links[0].LinkType)
	}
	if links[0].HopIndex != 0 {
		t.Errorf("expected hop index 0, got %d", links[0].HopIndex)
	}
}

func TestParseAGEPath_invalidJSON(t *testing.T) {
	links := parseAGEPath([]byte(`not valid json`))
	if links != nil {
		t.Errorf("expected nil for invalid JSON, got %v", links)
	}
}

func TestParseAGEPath_tooShort(t *testing.T) {
	raw := []byte(`[{"id":"memory.1"}::vertex]::path`)
	links := parseAGEPath(raw)
	if links != nil {
		t.Errorf("expected nil for path with <3 elements, got %v", links)
	}
}

func TestFindRelated_normalization(t *testing.T) {
	g := &GraphClient{db: nil} // No DB → will return early after normalization
	result, err := g.FindRelated(t.Context(), "t1", 1, nil, 0, -1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// With nil DB, IsAvailable returns false, so we get an empty result.
	// The normalization should have run successfully (no panic).
}

func TestFindChain_normalization(t *testing.T) {
	g := &GraphClient{db: nil}
	result, err := g.FindChain(t.Context(), "t1", 1, 2, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
