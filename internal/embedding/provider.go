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
	cfg := openai.DefaultConfig(apiKey)
	return NewOpenAIProviderWithConfig(cfg, model)
}

// NewOpenAIProviderWithConfig returns a Provider using the given OpenAI client config.
// Use in tests by setting cfg.BaseURL to an httptest server.
func NewOpenAIProviderWithConfig(cfg openai.ClientConfig, model string) Provider {
	if model == "" {
		model = string(openai.SmallEmbedding3)
	}
	return &OpenAIProvider{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

func (p *OpenAIProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	resp, err := p.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input:      []string{text},
		Model:      openai.EmbeddingModel(p.model),
		Dimensions: 1536, // ← FORCED TO 1536 for DB compatibility
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding error: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}
	return resp.Data[0].Embedding, nil
}
