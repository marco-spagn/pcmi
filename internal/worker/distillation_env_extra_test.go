package worker

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestDistillationBatchSizeTrimsWhitespace(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "  25  ")
	cfg := config.Load()
	if got := distillationBatchSizeFrom(cfg.DistillationBatchSize); got != 25 {
		t.Fatalf("expected 25, got %d", got)
	}
}

func TestDistillationBatchSizeNonNumeric(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "not-a-number")
	cfg := config.Load()
	if got := distillationBatchSizeFrom(cfg.DistillationBatchSize); got != defaultDistillationBatchSize {
		t.Fatalf("non-numeric must fall back, got %d", got)
	}
}

func TestDistillationBatchSizeBoundaryValues(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "1")
	cfg := config.Load()
	if got := distillationBatchSizeFrom(cfg.DistillationBatchSize); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	t.Setenv("DISTILLATION_BATCH_SIZE", "200")
	cfg = config.Load()
	if got := distillationBatchSizeFrom(cfg.DistillationBatchSize); got != 200 {
		t.Fatalf("expected 200, got %d", got)
	}
}

func TestDistillationConcurrencyTrimsWhitespace(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "  12  ")
	cfg := config.Load()
	if got := distillationConcurrencyFrom(cfg.DistillationConcurrency); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
}

func TestDistillationConcurrencyNonNumeric(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "abc")
	cfg := config.Load()
	if got := distillationConcurrencyFrom(cfg.DistillationConcurrency); got != defaultDistillationConcurrency {
		t.Fatalf("non-numeric must fall back, got %d", got)
	}
}

func TestDistillationConcurrencyOutOfRangeLow(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "0")
	cfg := config.Load()
	if got := distillationConcurrencyFrom(cfg.DistillationConcurrency); got != defaultDistillationConcurrency {
		t.Fatalf("0 must fall back (min=1), got %d", got)
	}
}

func TestDistillationConcurrencyBoundary(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "1")
	cfg := config.Load()
	if got := distillationConcurrencyFrom(cfg.DistillationConcurrency); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	t.Setenv("DISTILLATION_CONCURRENCY", "16")
	cfg = config.Load()
	if got := distillationConcurrencyFrom(cfg.DistillationConcurrency); got != 16 {
		t.Fatalf("expected 16, got %d", got)
	}
}
