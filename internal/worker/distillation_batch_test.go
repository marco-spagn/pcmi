package worker

import (
	"testing"
)

func TestDistillationBatchSizeDefault(t *testing.T) {
	if got := distillationBatchSizeFrom(0); got != defaultDistillationBatchSize {
		t.Fatalf("expected default %d, got %d", defaultDistillationBatchSize, got)
	}
}

func TestDistillationBatchSizeCustom(t *testing.T) {
	if got := distillationBatchSizeFrom(50); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}

func TestDistillationBatchSizeInvalidString(t *testing.T) {
	if got := distillationBatchSizeFrom(-1); got != defaultDistillationBatchSize {
		t.Fatalf("expected default %d for invalid, got %d", defaultDistillationBatchSize, got)
	}
}

func TestDistillationBatchSizeOutOfRangeLow(t *testing.T) {
	if got := distillationBatchSizeFrom(0); got != defaultDistillationBatchSize {
		t.Fatalf("expected default for 0, got %d", got)
	}
}

func TestDistillationBatchSizeOutOfRangeHigh(t *testing.T) {
	if got := distillationBatchSizeFrom(201); got != defaultDistillationBatchSize {
		t.Fatalf("expected default for 201, got %d", got)
	}
}

func TestDistillationBatchSizeBoundary(t *testing.T) {
	if got := distillationBatchSizeFrom(1); got != 1 {
		t.Fatalf("expected 1 (boundary low), got %d", got)
	}
	if got := distillationBatchSizeFrom(200); got != 200 {
		t.Fatalf("expected 200 (boundary high), got %d", got)
	}
}
