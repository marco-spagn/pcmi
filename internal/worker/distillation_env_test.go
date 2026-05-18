package worker

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestDistillationBatchSize_defaultAndEnv(t *testing.T) {
	t.Setenv("DISTILLATION_BATCH_SIZE", "")
	cfg := config.Load()
	if distillationBatchSizeFrom(cfg.DistillationBatchSize) != defaultDistillationBatchSize {
		t.Fatal("default batch")
	}
	t.Setenv("DISTILLATION_BATCH_SIZE", "25")
	cfg = config.Load()
	if distillationBatchSizeFrom(cfg.DistillationBatchSize) != 25 {
		t.Fatal("env batch")
	}
	t.Setenv("DISTILLATION_BATCH_SIZE", "9999")
	cfg = config.Load()
	if got := distillationBatchSizeFrom(cfg.DistillationBatchSize); got != defaultDistillationBatchSize {
		t.Fatalf("invalid should default, got %d", got)
	}
}

func TestDistillationConcurrency_defaultAndEnv(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "")
	cfg := config.Load()
	if distillationConcurrencyFrom(cfg.DistillationConcurrency) != defaultDistillationConcurrency {
		t.Fatal("default concurrency")
	}
	t.Setenv("DISTILLATION_CONCURRENCY", "8")
	cfg = config.Load()
	if distillationConcurrencyFrom(cfg.DistillationConcurrency) != 8 {
		t.Fatal("env concurrency")
	}
	t.Setenv("DISTILLATION_CONCURRENCY", "100")
	cfg = config.Load()
	if got := distillationConcurrencyFrom(cfg.DistillationConcurrency); got != defaultDistillationConcurrency {
		t.Fatalf("invalid should default, got %d", got)
	}
}
