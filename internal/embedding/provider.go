package embedding

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

type Provider interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

type OpenAIProvider struct {
	client *openai.Client
	model  string
}

func NewOpenAIProvider(apiKey, model string) Provider {
	return &OpenAIProvider{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (p *OpenAIProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	resp, err := p.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(p.model),
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding error: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return resp.Data[0].Embedding, nil
}
