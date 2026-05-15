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
	if model == "" {
		model = string(openai.SmallEmbedding3)
	}
	return &OpenAIProvider{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (p *OpenAIProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	resp, err := p.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input:      []string{text},
		Model:      openai.EmbeddingModel(p.model),
		Dimensions: 1536, // ← FORZATO A 1536 per compatibilità con DB
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding error: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}
	return resp.Data[0].Embedding, nil
}
