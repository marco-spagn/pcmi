package worker

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// openAILLMClient wraps the go-openai SDK and satisfies LLMClient.
// It is reused for OpenAI, Grok (api.x.ai), and DeepSeek (api.deepseek.com)
// because all three expose an OpenAI-compatible /v1/chat/completions endpoint.
type openAILLMClient struct {
	client    *openai.Client
	modelName string
	apiKey    string
}

func (c *openAILLMClient) IsConfigured() bool { return c.apiKey != "" }

func (c *openAILLMClient) Complete(ctx context.Context, systemPrompt string, userMessages []string) (string, error) {
	var msgs []openai.ChatCompletionMessage
	if systemPrompt != "" {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}
	for _, m := range userMessages {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: m,
		})
	}
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    c.modelName,
		Messages: msgs,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned 0 choices (finish_reason may be content_filter)")
	}
	return resp.Choices[0].Message.Content, nil
}

// newOpenAIClient builds a client targeting the default OpenAI endpoint.
func newOpenAIClient(apiKey, model string) *openAILLMClient {
	cfg := openai.DefaultConfig(apiKey)
	return &openAILLMClient{client: openai.NewClientWithConfig(cfg), modelName: model, apiKey: apiKey}
}

// newGrokClient builds a client targeting the xAI Grok endpoint.
// Grok exposes an OpenAI-compatible API at https://api.x.ai/v1.
func newGrokClient(apiKey, model string) *openAILLMClient {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = "https://api.x.ai/v1"
	return &openAILLMClient{client: openai.NewClientWithConfig(cfg), modelName: model, apiKey: apiKey}
}

// newDeepSeekClient builds a client targeting the DeepSeek endpoint.
// DeepSeek exposes an OpenAI-compatible API at https://api.deepseek.com/v1.
func newDeepSeekClient(apiKey, model string) *openAILLMClient {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = "https://api.deepseek.com/v1"
	return &openAILLMClient{client: openai.NewClientWithConfig(cfg), modelName: model, apiKey: apiKey}
}
