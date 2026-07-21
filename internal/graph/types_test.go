package graph

import "testing"

func TestParseTraversalDirection(t *testing.T) {
	tests := []struct {
		in   string
		want TraversalDirection
		ok   bool
	}{
		{"", TraversalBoth, true},
		{"both", TraversalBoth, true},
		{"OUT", TraversalOut, true},
		{"in", TraversalIn, true},
		{"sideways", "", false},
	}
	for _, tt := range tests {
		got, err := ParseTraversalDirection(tt.in)
		if tt.ok && err != nil {
			t.Fatalf("ParseTraversalDirection(%q): %v", tt.in, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("ParseTraversalDirection(%q): want error", tt.in)
		}
		if tt.ok && got != tt.want {
			t.Fatalf("ParseTraversalDirection(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildTraversePattern(t *testing.T) {
	if got := buildTraversePattern(3, "r", TraversalOut); got != "-[r*1..3]->" {
		t.Fatalf("out = %q", got)
	}
	if got := buildTraversePattern(2, "r", TraversalIn); got != "<-[r*1..2]-" {
		t.Fatalf("in = %q", got)
	}
	if got := buildTraversePattern(4, "r", TraversalBoth); got != "-[r*1..4]-" {
		t.Fatalf("both = %q", got)
	}
}
