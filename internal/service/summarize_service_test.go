package service

import "testing"

func TestExtractiveSummaryBrief(t *testing.T) {
	parts := []string{"alpha memory content", "beta memory content"}
	s := extractiveSummary(parts, "brief")
	if s == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestExtractiveSummaryDetailedLonger(t *testing.T) {
	long := make([]byte, 2000)
	for i := range long {
		long[i] = 'x'
	}
	brief := extractiveSummary([]string{string(long)}, "brief")
	detailed := extractiveSummary([]string{string(long)}, "detailed")
	if len(detailed) <= len(brief) {
		t.Fatalf("detailed should allow more chars: brief=%d detailed=%d", len(brief), len(detailed))
	}
}
