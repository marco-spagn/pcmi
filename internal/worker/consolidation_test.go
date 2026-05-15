package worker

import "testing"

func TestParentPrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"root.a.b.c", "root.a.b"},
		{"root.only", "root"},
		{"root", "root"},
		{"", "root"},
	}
	for _, tc := range tests {
		if got := parentPrefix(tc.in); got != tc.want {
			t.Fatalf("parentPrefix(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
