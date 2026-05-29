package main

import "testing"

func TestMin_Worker(t *testing.T) {
	t.Parallel()

	cases := []struct{ a, b, want int }{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 5, -1},
		{5, -1, -1},
		{100, 100, 100},
	}

	for _, tc := range cases {
		if got := min(tc.a, tc.b); got != tc.want {
			t.Errorf("min(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
