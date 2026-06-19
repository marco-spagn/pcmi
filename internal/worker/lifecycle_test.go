package worker

import (
	"context"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestNewDistillationWorker_withConfig(t *testing.T) {
	cfg := &config.Config{
		OpenAIAPIKey:            "sk-test",
		DistillationModel:       " custom-model ",
		DistillationBatchSize:   25,
		DistillationConcurrency: 8,
	}
	w := NewDistillationWorker(nil, cfg)
	llm, ok := w.llm.(*openAILLMClient)
	if !ok {
		t.Fatalf("expected *openAILLMClient, got %T", w.llm)
	}
	if llm.apiKey != "sk-test" {
		t.Fatalf("apiKey=%q", llm.apiKey)
	}
	if llm.modelName != "custom-model" {
		t.Fatalf("modelName=%q", llm.modelName)
	}
	if w.batchSize != 25 {
		t.Fatalf("batchSize=%d", w.batchSize)
	}
	if cap(w.sem) != 8 {
		t.Fatalf("sem cap=%d", cap(w.sem))
	}
}

func TestNewDistillationWorker_nilConfigUsesDefaults(t *testing.T) {
	w := NewDistillationWorker(nil, nil)
	if w.batchSize != defaultDistillationBatchSize {
		t.Fatalf("batchSize=%d", w.batchSize)
	}
	llm, ok := w.llm.(*openAILLMClient)
	if !ok {
		t.Fatalf("expected *openAILLMClient, got %T", w.llm)
	}
	if llm.modelName != defaultModelOpenAI {
		t.Fatalf("modelName=%q", llm.modelName)
	}
	if cap(w.sem) != defaultDistillationConcurrency {
		t.Fatalf("sem cap=%d", cap(w.sem))
	}
}

func TestDistillationWorker_Start_exitsOnCancel(t *testing.T) {
	w := NewDistillationWorker(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestExpiryWorker_Start_exitsOnCancel(t *testing.T) {
	w := NewExpiryWorker(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}
