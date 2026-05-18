package worker

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/embedding"
)

func TestNewEmbeddingWorker(t *testing.T) {
	w := NewEmbeddingWorker(nil, nil)
	if w == nil || w.db != nil || w.provider != nil {
		t.Fatalf("%+v", w)
	}
	w2 := NewEmbeddingWorker(nil, embedding.NewOpenAIProvider("k", "m"))
	if w2 == nil || w2.provider == nil {
		t.Fatal("expected provider set")
	}
}
