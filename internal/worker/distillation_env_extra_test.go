package worker

import "testing"

// Extra coverage on top of distillation_env_test.go: boundary values, whitespace
// trimming, non-numeric input, lower-bound rejection. Pure functions — no DB.

func TestDistillationBatchSizeTrimsWhitespace(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "  25  ")
	if got := distillationBatchSize(); got != 25 {
		t.Fatalf("expected 25, got %d", got)
	}
}

func TestDistillationBatchSizeNonNumeric(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "not-a-number")
	if got := distillationBatchSize(); got != defaultDistillationBatchSize {
		t.Fatalf("non-numeric must fall back, got %d", got)
	}
}

func TestDistillationBatchSizeBoundaryValues(t *testing.T) {
	// Min boundary
	t.Setenv("DISTILLATION_BATCH_SIZE", "1")
	if got := distillationBatchSize(); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	// Max boundary
	t.Setenv("DISTILLATION_BATCH_SIZE", "200")
	if got := distillationBatchSize(); got != 200 {
		t.Fatalf("expected 200, got %d", got)
	}
}

func TestDistillationConcurrencyTrimsWhitespace(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "  12  ")
	if got := distillationConcurrency(); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
}

func TestDistillationConcurrencyNonNumeric(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "abc")
	if got := distillationConcurrency(); got != defaultDistillationConcurrency {
		t.Fatalf("non-numeric must fall back, got %d", got)
	}
}

func TestDistillationConcurrencyOutOfRangeLow(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "0")
	if got := distillationConcurrency(); got != defaultDistillationConcurrency {
		t.Fatalf("0 must fall back (min=1), got %d", got)
	}
}

func TestDistillationConcurrencyBoundary(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "1")
	if got := distillationConcurrency(); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	t.Setenv("DISTILLATION_CONCURRENCY", "16")
	if got := distillationConcurrency(); got != 16 {
		t.Fatalf("expected 16, got %d", got)
	}
}
