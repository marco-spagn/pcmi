package worker

import "testing"

func TestDistillPathPrefix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "root.test"},
		{"root.test.foo", "root.test"},
		{"root.ci.smoke", "root.ci"},
		{"solo", "solo"},
	}
	for _, tc := range tests {
		if got := DistillPathPrefix(tc.in); got != tc.want {
			t.Errorf("DistillPathPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSourceIDsEqual(t *testing.T) {
	if !sourceIDsEqual([]int64{3, 1, 2}, []int64{2, 3, 1}) {
		t.Fatal("expected equal source id sets")
	}
	if sourceIDsEqual([]int64{1}, []int64{1, 2}) {
		t.Fatal("expected different lengths to be unequal")
	}
}
