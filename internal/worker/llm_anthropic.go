package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	anthropicMaxTokens  = 2048
)

// anthropicLLMClient calls the Anthropic /v1/messages endpoint directly via
// HTTP without adding the anthropic-sdk-go as a dependency.  The response
// format is different from OpenAI: the system prompt is a top-level field
// and content arrives in a typed array.
type anthropicLLMClient struct {
	apiKey     string
	modelName  string
	httpClient *http.Client
}

func newAnthropicClient(apiKey, model string) *anthropicLLMClient {
	return &anthropicLLMClient{
		apiKey:    apiKey,
		modelName: model,
		httpClient: &http.Client{Timeout: 3 * time.Minute},
	}
}

func (c *anthropicLLMClient) IsConfigured() bool { return c.apiKey != "" }

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *anthropicLLMClient) Complete(ctx context.Context, systemPrompt string, userMessages []string) (string, error) {
	msgs := make([]anthropicMessage, 0, len(userMessages))
	for _, m := range userMessages {
		msgs = append(msgs, anthropicMessage{Role: "user", Content: m})
	}

	body, err := json.Marshal(anthropicRequest{
		Model:     c.modelName,
		MaxTokens: anthropicMaxTokens,
		System:    systemPrompt,
		Messages:  msgs,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, raw)
	}

	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return "", fmt.Errorf("anthropic unmarshal: %w", err)
	}
	if ar.Error != nil {
		return "", fmt.Errorf("anthropic API error %s: %s", ar.Error.Type, ar.Error.Message)
	}
	for _, block := range ar.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text block in response")
}
