package worker

import "context"

// LLMClient is the single call site that DistillationWorker needs: given a
// system prompt and a list of user messages, return the model's text response.
// The interface is intentionally narrow — callers never need streaming or
// multi-turn back-and-forth, so a single Complete is enough.
//
// Implementations: openAILLMClient (OpenAI, Grok, DeepSeek — all OpenAI-
// compatible), anthropicLLMClient (Claude — native /v1/messages API).
type LLMClient interface {
	// Complete sends a single chat-completion request and returns the first
	// choice's text.  systemPrompt may be empty.  userMessages is the ordered
	// list of user-turn contents.
	Complete(ctx context.Context, systemPrompt string, userMessages []string) (string, error)

	// IsConfigured reports whether the client has a non-empty API key and can
	// actually make calls.  DistillationWorker skips LLM work when false.
	IsConfigured() bool
}
