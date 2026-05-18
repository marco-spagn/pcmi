package embedding

import (
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestNewOpenAIProvider_defaultModel(t *testing.T) {
	p := NewOpenAIProvider("k", "")
	op, ok := p.(*OpenAIProvider)
	if !ok || op.model != string(openai.SmallEmbedding3) {
		t.Fatalf("got %#v ok=%v", p, ok)
	}
}

func TestNewOpenAIProvider_explicitModel(t *testing.T) {
	p := NewOpenAIProvider("k", "text-embedding-3-large")
	op, ok := p.(*OpenAIProvider)
	if !ok || op.model != "text-embedding-3-large" {
		t.Fatalf("model = %q ok=%v", op.model, ok)
	}
}
