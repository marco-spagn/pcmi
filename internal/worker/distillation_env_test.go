package worker

import (
	"testing"
)

func TestDistillationBatchSize_defaultAndEnv(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "")
	if distillationBatchSize() != defaultDistillationBatchSize {
		t.Fatal("default batch")
	}
	t.Setenv("DISTILLATION_BATCH_SIZE", "25")
	if distillationBatchSize() != 25 {
		t.Fatal("env batch")
	}
	t.Setenv("DISTILLATION_BATCH_SIZE", "9999")
	if got := distillationBatchSize(); got != defaultDistillationBatchSize {
		t.Fatalf("invalid should default, got %d", got)
	}
}

func TestDistillationConcurrency_defaultAndEnv(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "")
	if distillationConcurrency() != defaultDistillationConcurrency {
		t.Fatal("default concurrency")
	}
	t.Setenv("DISTILLATION_CONCURRENCY", "8")
	if distillationConcurrency() != 8 {
		t.Fatal("env concurrency")
	}
	t.Setenv("DISTILLATION_CONCURRENCY", "100")
	if got := distillationConcurrency(); got != defaultDistillationConcurrency {
		t.Fatalf("invalid should default, got %d", got)
	}
}
