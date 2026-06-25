package model

import "time"

// AgentSession is a bounded working-memory context for one agent run.
type AgentSession struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	AgentID   *string        `json:"agent_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
	Status    string         `json:"status"` // active | ended
}

// CreateSessionRequest starts a new agent session.
type CreateSessionRequest struct {
	AgentID  string         `json:"agent_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SessionStoreMemoryRequest stores working memory under a session.
type SessionStoreMemoryRequest struct {
	Path           string         `json:"path" validate:"required"`
	Content        string         `json:"content" validate:"required"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	EmbeddingModel string         `json:"embedding_model,omitempty"`
	SourceAgentID  string         `json:"source_agent_id,omitempty"`
	Importance     *float64       `json:"importance,omitempty"`
}

// SessionMemoriesResponse lists working (and optionally long-term) memories.
type SessionMemoriesResponse struct {
	SessionID string        `json:"session_id"`
	Entries   []MemoryEntry `json:"entries"`
	Total     int           `json:"total"`
}

// PromoteSessionRequest copies session working memory into long-term paths.
type PromoteSessionRequest struct {
	// TargetPrefix is the ltree prefix for promoted rows (default "root").
	TargetPrefix string `json:"target_prefix,omitempty"`
}

// PromoteSessionResponse summarizes promotion results.
type PromoteSessionResponse struct {
	SessionID    string `json:"session_id"`
	Promoted     int    `json:"promoted"`
	Skipped      int    `json:"skipped"` // target path already had a current memory
	TargetPrefix string `json:"target_prefix"`
	Status       string `json:"status"`
}
