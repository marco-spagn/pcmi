package model

import (
	"testing"
)

func TestNormalizeImportance(t *testing.T) {
	t.Parallel()

	// nil returns default
	if got := NormalizeImportance(nil); got != 0.5 {
		t.Fatalf("nil: got %v, want 0.5", got)
	}

	// negative → 0
	neg := -0.3
	if got := NormalizeImportance(&neg); got != 0 {
		t.Fatalf("-0.3: got %v, want 0", got)
	}

	// > 1 → 1
	big := 2.5
	if got := NormalizeImportance(&big); got != 1 {
		t.Fatalf("2.5: got %v, want 1", got)
	}

	// within range
	mid := 0.75
	if got := NormalizeImportance(&mid); got != 0.75 {
		t.Fatalf("0.75: got %v, want 0.75", got)
	}

	// boundary 0
	zero := 0.0
	if got := NormalizeImportance(&zero); got != 0 {
		t.Fatalf("0.0: got %v, want 0", got)
	}

	// boundary 1
	one := 1.0
	if got := NormalizeImportance(&one); got != 1 {
		t.Fatalf("1.0: got %v, want 1", got)
	}
}

func TestValidateImportance(t *testing.T) {
	t.Parallel()

	if err := ValidateImportance(0.5); err != nil {
		t.Fatalf("0.5 should be valid, got: %v", err)
	}
	if err := ValidateImportance(0); err != nil {
		t.Fatalf("0 should be valid, got: %v", err)
	}
	if err := ValidateImportance(1); err != nil {
		t.Fatalf("1 should be valid, got: %v", err)
	}
	if err := ValidateImportance(-0.01); err == nil {
		t.Fatal("negative should be invalid")
	}
	if err := ValidateImportance(1.01); err == nil {
		t.Fatal(">1 should be invalid")
	}
}

func TestDedupLinkType(t *testing.T) {
	t.Parallel()
	if got := DedupLinkType(); got == "" {
		t.Fatal("DedupLinkType should not be empty")
	}
}
