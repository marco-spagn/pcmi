package worker

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8(t *testing.T) {
	// "é" is 2 bytes (0xC3 0xA9); a byte-cut at an odd offset would split it.
	multi := "abcé" // 5 bytes: a b c 0xC3 0xA9
	euro := "€€€"   // 9 bytes: 3×(0xE2 0x82 0xAC)

	cases := []struct {
		name     string
		in       string
		maxBytes int
		want     string
	}{
		{"under limit", "hello", 10, "hello"},
		{"exact limit", "hello", 5, "hello"},
		{"ascii cut on boundary", "hello", 3, "hel"},
		{"cut splits 2-byte rune", multi, 4, "abc"}, // would-be "abc\xc3" → drop partial
		{"cut on rune boundary", multi, 3, "abc"},
		{"cut mid 3-byte rune", euro, 4, "€"},   // 3 bytes kept, 4th byte is mid-rune
		{"cut mid 3-byte rune 2", euro, 5, "€"}, // bytes 4-5 are mid second rune
		{"cut on 3-byte boundary", euro, 6, "€€"},
		{"zero", "abc", 0, ""},
		{"negative", "abc", -1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateUTF8(tc.in, tc.maxBytes)
			if got != tc.want {
				t.Fatalf("truncateUTF8(%q, %d) = %q, want %q", tc.in, tc.maxBytes, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("truncateUTF8(%q, %d) produced invalid UTF-8: %q", tc.in, tc.maxBytes, got)
			}
			if len(got) > tc.maxBytes && tc.maxBytes > 0 {
				t.Fatalf("truncateUTF8(%q, %d) exceeded byte budget: %d bytes", tc.in, tc.maxBytes, len(got))
			}
		})
	}
}

func TestTruncateUTF8_AlwaysValidUnderByteCuts(t *testing.T) {
	// Cutting a multibyte-heavy string at every byte offset must never yield
	// invalid UTF-8.
	s := "héllo wörld €uro 日本語"
	for n := 0; n <= len(s)+2; n++ {
		if got := truncateUTF8(s, n); !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 at maxBytes=%d: %q", n, got)
		}
	}
}
