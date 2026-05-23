package model

import "testing"

func TestContentHash_normalizationCaseInsensitive(t *testing.T) {
	t.Parallel()
	a := ContentHash("Hello World")
	b := ContentHash("hello world")
	if a != b {
		t.Fatalf("expected equal hashes, got %q vs %q", a, b)
	}
}

func TestContentHash_normalizationUnicode(t *testing.T) {
	t.Parallel()
	// é as single code point vs e + combining acute
	a := ContentHash("café")
	b := ContentHash("caf\u00e9")
	c := ContentHash("cafe\u0301")
	if a != b || b != c {
		t.Fatalf("NFC mismatch: %q %q %q", a, b, c)
	}
}

func TestContentHash_emptyAndWhitespaceOnly(t *testing.T) {
	t.Parallel()
	empty := ContentHash("")
	ws := ContentHash("   \t\n  ")
	if empty != ws {
		t.Fatalf("empty and whitespace-only should match: %q vs %q", empty, ws)
	}
	if empty == ContentHash("x") {
		t.Fatal("empty hash should differ from non-empty content")
	}
}

func TestNormalizeContentForHash_emptyAndWhitespace(t *testing.T) {
	t.Parallel()
	if got := NormalizeContentForHash("  \t  "); got != "" {
		t.Fatalf("whitespace-only should normalize to empty, got %q", got)
	}
}

func TestParseDedupMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want DedupMode
		err  bool
	}{
		{"skip", DedupModeSkip, false},
		{"LINK", DedupModeLink, false},
		{"", DedupModeNone, false},
		{"bogus", "", true},
	} {
		got, err := ParseDedupMode(tc.in)
		if tc.err {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %q err=%v", tc.in, got, err)
		}
	}
}
