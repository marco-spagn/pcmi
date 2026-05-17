package service

import (
	"strings"
	"testing"
)

func TestExtractiveSummaryShort(t *testing.T) {
	parts := []string{"hello", "world"}
	got := extractiveSummary(parts, "")
	if got != "hello\n\nworld" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestExtractiveSummaryTruncated(t *testing.T) {
	// Build a string longer than 400 characters
	longPart := strings.Repeat("a", 500)
	got := extractiveSummary([]string{longPart}, "")
	if len(got) > 405 { // 400 + "…"
		t.Fatalf("summary not truncated: len=%d", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis at end of truncated summary, got: %q", got[:20])
	}
}

func TestExtractiveSummaryDetailed(t *testing.T) {
	longPart := strings.Repeat("b", 1100)
	got := extractiveSummary([]string{longPart}, "detailed")
	// 1200 chars limit for detailed
	if len(got) > 1205 {
		t.Fatalf("detailed summary too long: %d", len(got))
	}
}

func TestExtractiveSummaryDetailedNoTrunc(t *testing.T) {
	part := strings.Repeat("c", 50)
	got := extractiveSummary([]string{part}, "Detailed") // case-insensitive
	if got != part {
		t.Fatalf("expected no truncation for short detailed summary")
	}
}

func TestExtractiveSummaryEmpty(t *testing.T) {
	got := extractiveSummary(nil, "")
	if got != "" {
		t.Fatalf("expected empty summary for nil parts, got %q", got)
	}
}
