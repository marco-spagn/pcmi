package main

import "testing"

func TestMin(t *testing.T) {
	if min(1, 100) != 1 || min(50, 20) != 20 {
		t.Fatalf("min: %d %d", min(1, 100), min(50, 20))
	}
}
