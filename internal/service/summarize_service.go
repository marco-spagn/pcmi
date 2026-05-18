package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type SummarizeService struct {
	repo repository.MemoryRepo
}

func NewSummarizeService(repo repository.MemoryRepo) *SummarizeService {
	return &SummarizeService{repo: repo}
}

type SummarizeRequest struct {
	PathPrefix string `json:"path_prefix"`
	Limit      int    `json:"limit"`
	Style      string `json:"style"`
}

type SummarizeResponse struct {
	PathPrefix string `json:"path_prefix"`
	Summary    string `json:"summary"`
	SourceIDs  []int64 `json:"source_ids"`
	Method     string `json:"method"`
	Total      int    `json:"total"`
}

func (s *SummarizeService) Summarize(ctx context.Context, req *SummarizeRequest, tenantID string) (*SummarizeResponse, error) {
	prefix := strings.TrimSpace(req.PathPrefix)
	if prefix == "" {
		prefix = "root"
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	retrieveReq := model.RetrieveRequest{
		PathPrefix: prefix,
		Limit:      limit,
	}
	entries, err := s.repo.Retrieve(ctx, retrieveReq, tenantID, nil)
	if err != nil {
		return nil, fmt.Errorf("summarize retrieve: %w", err)
	}
	if len(entries) == 0 {
		return &SummarizeResponse{
			PathPrefix: prefix,
			Summary:    "",
			SourceIDs:  []int64{},
			Method:     "none",
			Total:      0,
		}, nil
	}

	ids := make([]int64, 0, len(entries))
	var parts []string
	for _, e := range entries {
		ids = append(ids, e.ID)
		c := strings.TrimSpace(e.Content)
		if c != "" {
			parts = append(parts, c)
		}
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		summary, err := llmSummarize(ctx, key, parts, req.Style)
		if err == nil && summary != "" {
			return &SummarizeResponse{
				PathPrefix: prefix,
				Summary:    summary,
				SourceIDs:  ids,
				Method:     "llm",
				Total:      len(entries),
			}, nil
		}
	}

	return &SummarizeResponse{
		PathPrefix: prefix,
		Summary:    extractiveSummary(parts, req.Style),
		SourceIDs:  ids,
		Method:     "extractive",
		Total:      len(entries),
	}, nil
}

func extractiveSummary(parts []string, style string) string {
	combined := strings.Join(parts, "\n\n")
	maxLen := 400
	if strings.EqualFold(style, "detailed") {
		maxLen = 1200
	}
	if len(combined) <= maxLen {
		return combined
	}
	return combined[:maxLen] + "…"
}

func llmSummarize(ctx context.Context, apiKey string, parts []string, style string) (string, error) {
	client := openai.NewClient(apiKey)
	modelName := os.Getenv("DISTILLATION_MODEL")
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}
	instruction := "Summarize the following memories into a concise paragraph."
	if strings.EqualFold(style, "detailed") {
		instruction = "Summarize the following memories into a detailed multi-paragraph overview."
	}
	var messages []openai.ChatCompletionMessage
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: instruction + " Return plain text only.",
	})
	for _, p := range parts {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: p,
		})
	}
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty LLM response")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
