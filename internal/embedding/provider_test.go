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
