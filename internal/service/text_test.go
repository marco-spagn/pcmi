package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8_NeverSplitsRune(t *testing.T) {
	s := "café — 日本語 — €uro"
	for n := 0; n <= len(s)+2; n++ {
		got := truncateUTF8(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 at maxBytes=%d: %q", n, got)
		}
		if n > 0 && len(got) > n {
			t.Fatalf("maxBytes=%d exceeded: %d bytes", n, len(got))
		}
	}
	if got := truncateUTF8("abc", 0); got != "" {
		t.Fatalf("maxBytes=0 should be empty, got %q", got)
	}
}

func TestExtractiveSummary_ValidUTF8WhenTruncated(t *testing.T) {
	// A long run of multibyte runes forces truncation at maxLen=400; the result
	// (plus the appended ellipsis) must remain valid UTF-8.
	parts := []string{strings.Repeat("日", 500)} // 1500 bytes
	got := extractiveSummary(parts, "")
	if !utf8.ValidString(got) {
		t.Fatalf("extractiveSummary produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix on truncated summary, got %q", got)
	}
}
