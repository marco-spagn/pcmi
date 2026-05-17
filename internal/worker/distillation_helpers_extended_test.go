package worker

import (
	"testing"
)

func TestNormalizeSourceIDs(t *testing.T) {
	in := []int64{5, 3, 1, 2, 4}
	got := normalizeSourceIDs(in)
	want := []int64{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d want %d", i, got[i], want[i])
		}
	}
	// original should be unchanged (copy semantics)
	if in[0] != 5 {
		t.Fatal("normalizeSourceIDs must not mutate the input slice")
	}
}

func TestNormalizeSourceIDsEmpty(t *testing.T) {
	got := normalizeSourceIDs(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty slice for nil input, got %v", got)
	}
}

func TestSourceIDsEqualSameElements(t *testing.T) {
	a := []int64{10, 20, 30}
	b := []int64{30, 10, 20}
	if !sourceIDsEqual(a, b) {
		t.Fatal("expected equal for same elements in different order")
	}
}

func TestSourceIDsEqualDifferentLengths(t *testing.T) {
	if sourceIDsEqual([]int64{1, 2}, []int64{1}) {
		t.Fatal("different lengths must not be equal")
	}
}

func TestSourceIDsEqualDifferentValues(t *testing.T) {
	if sourceIDsEqual([]int64{1, 2, 3}, []int64{1, 2, 4}) {
		t.Fatal("different values must not be equal")
	}
}

func TestSourceIDsEqualBothEmpty(t *testing.T) {
	if !sourceIDsEqual(nil, []int64{}) {
		t.Fatal("two empty slices must be equal")
	}
}

func TestDistillPathPrefixEdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  ", "root.test"},             // whitespace-only
		{"a", "a"},                      // single segment
		{"a.b.c.d", "a.b"},             // deep path: take first two
		{"root.test.deep.path", "root.test"}, // root.test prefix preserved
	}
	for _, tc := range cases {
		if got := DistillPathPrefix(tc.in); got != tc.want {
			t.Errorf("DistillPathPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
