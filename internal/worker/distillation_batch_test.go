package worker

import (
	"testing"
)

func TestDistillationBatchSizeDefault(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "")
	if got := distillationBatchSize(); got != defaultDistillationBatchSize {
		t.Fatalf("expected default %d, got %d", defaultDistillationBatchSize, got)
	}
}

func TestDistillationBatchSizeCustom(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "50")
	if got := distillationBatchSize(); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}

func TestDistillationBatchSizeInvalidString(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "nope")
	if got := distillationBatchSize(); got != defaultDistillationBatchSize {
		t.Fatalf("expected default %d for invalid string, got %d", defaultDistillationBatchSize, got)
	}
}

func TestDistillationBatchSizeOutOfRangeLow(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "0")
	if got := distillationBatchSize(); got != defaultDistillationBatchSize {
		t.Fatalf("expected default for 0, got %d", got)
	}
}

func TestDistillationBatchSizeOutOfRangeHigh(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "201")
	if got := distillationBatchSize(); got != defaultDistillationBatchSize {
		t.Fatalf("expected default for 201, got %d", got)
	}
}

func TestDistillationBatchSizeBoundary(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "1")
	if got := distillationBatchSize(); got != 1 {
		t.Fatalf("expected 1 (boundary low), got %d", got)
	}

	t.Setenv("DISTILLATION_BATCH_SIZE", "200")
	if got := distillationBatchSize(); got != 200 {
		t.Fatalf("expected 200 (boundary high), got %d", got)
	}
}
