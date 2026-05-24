package worker

import (
	"strings"
	"testing"
)

// TestDistillPathPrefix_DistilledPathDoesNotIncludeItself verifies BUG-FIX-2:
// a path that ends in .distilled should still map to the parent prefix so the
// SQL NOT-lquery filter is consistent.
func TestDistillPathPrefix_DistilledPathDoesNotIncludeItself(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"root.security.alerts.sql_injection", "root.security"},
		{"root.security.distilled", "root.security"},
		{"root.trading.signals.engulf", "root.trading"},
		{"root.trading", "root.trading"},
		{"root", "root"},
		{"", "root.test"},
		{"root.test.sub", "root.test"},
	}
	for _, tc := range cases {
		got := DistillPathPrefix(tc.input)
		if got != tc.want {
			t.Errorf("DistillPathPrefix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestDistillationQueryExcludesDistilledPaths verifies BUG-FIX-2 uses ltree <@
// (not lquery) so PostgreSQL accepts the filter and .distilled branches are skipped.
func TestDistillationQueryExcludesDistilledPaths(t *testing.T) {
	sql := distillationSourceEntriesSQL()
	if strings.Contains(sql, "::lquery") {
		t.Fatal("distillation query must not use lquery; it caused ltree syntax errors in E2E")
	}
	const exclude = "NOT (path <@ (($2::text || '.distilled')::ltree))"
	if !strings.Contains(sql, exclude) {
		t.Fatalf("missing BUG-FIX-2 exclude clause:\n%s", sql)
	}
}

func TestDistilledBranchPath(t *testing.T) {
	cases := []struct{ prefix, want string }{
		{"root.test", "root.test.distilled"},
		{"root.security", "root.security.distilled"},
		{"root.sse", "root.sse.distilled"},
	}
	for _, tc := range cases {
		if got := distilledBranchPath(tc.prefix); got != tc.want {
			t.Errorf("distilledBranchPath(%q) = %q, want %q", tc.prefix, got, tc.want)
		}
	}
}

// TestParseDistillationLLMResponse_EmptyChoices verifies BUG-FIX-1:
// the JSON parser must not panic on empty LLM output.
func TestParseDistillationLLMResponse_EmptyChoices(t *testing.T) {
	// Simulate the model returning an empty string (e.g. content filter).
	_, err := parseDistillationLLMResponse("")
	if err == nil {
		t.Fatal("expected error for empty LLM response, got nil")
	}
}

// TestParseDistillationLLMResponse_ValidJSON verifies normal path is unaffected by BUG-FIX-1.
func TestParseDistillationLLMResponse_ValidJSON(t *testing.T) {
	raw := `{"summary":"test summary","insights":["a","b"]}`
	res, err := parseDistillationLLMResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "test summary" {
		t.Errorf("summary = %q", res.Summary)
	}
	if len(res.Insights) != 2 {
		t.Errorf("insights len = %d", len(res.Insights))
	}
}

// TestParseDistillationLLMResponse_CodeFences verifies sanitiser strips markdown fences.
func TestParseDistillationLLMResponse_CodeFences(t *testing.T) {
	raw := "```json\n{\"summary\":\"s\",\"insights\":[\"i\"]}\n```"
	res, err := parseDistillationLLMResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "s" {
		t.Errorf("summary = %q", res.Summary)
	}
}

// TestParseDistillationLLMResponse_TrailingComma verifies JSON repair path.
func TestParseDistillationLLMResponse_TrailingComma(t *testing.T) {
	raw := `{"summary":"s","insights":["a","b",]}`
	res, err := parseDistillationLLMResponse(raw)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if !strings.Contains(res.Summary, "s") {
		t.Errorf("summary = %q", res.Summary)
	}
}

// TestNormalizeSourceIDs_Stable verifies deterministic sort for dedup.
func TestNormalizeSourceIDs_Stable(t *testing.T) {
	in := []int64{5, 1, 3, 2, 4}
	got := normalizeSourceIDs(in)
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("not sorted at %d: %v", i, got)
		}
	}
	// Original must be unchanged.
	if in[0] != 5 {
		t.Errorf("original mutated: %v", in)
	}
}

// TestSourceIDsEqual_Order verifies dedup handles unsorted arrays.
func TestSourceIDsEqual_Order(t *testing.T) {
	a := []int64{3, 1, 2}
	b := []int64{1, 2, 3}
	if !sourceIDsEqual(a, b) {
		t.Error("expected equal regardless of order")
	}
	c := []int64{1, 2, 4}
	if sourceIDsEqual(a, c) {
		t.Error("expected not equal")
	}
}
