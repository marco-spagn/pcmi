package model

import "time"

type MemoryEntry struct {
	ID               int64      `json:"id"`
	TenantID         string     `json:"tenant_id"`
	Path             string     `json:"path"`
	Content          string     `json:"content"`
	Metadata         any        `json:"metadata"`
	Tags             []string   `json:"tags"`
	Embedding        []float32  `json:"embedding,omitempty"`
	EmbeddingModel   string     `json:"embedding_model"`
	EmbeddingSpace   string     `json:"embedding_space,omitempty"`
	Version          int        `json:"version"`
	ValidFrom        time.Time  `json:"valid_from"`
	ValidTo          *time.Time `json:"valid_to"`
	SourceAgentID    *string    `json:"source_agent_id,omitempty"`
	SourceEventID    *string    `json:"source_event_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	RelevanceScore    float64    `json:"relevance_score,omitempty"`
	ContentEncrypted  bool       `json:"content_encrypted,omitempty"`
}

type StoreRequest struct {
	TenantID       string                 `json:"tenant_id,omitempty"`
	Path           string                 `json:"path" validate:"required"`
	Content        string                 `json:"content" validate:"required"`
	Metadata       map[string]interface{} `json:"metadata"`
	Tags           []string               `json:"tags"`
	Embedding      []float32              `json:"embedding"`
	EmbeddingModel string                 `json:"embedding_model"`
	EmbeddingSpace string                 `json:"embedding_space"`
	SourceAgentID  string                 `json:"source_agent_id"`
	EncryptContent bool                   `json:"encrypt_content"`
}

type RetrieveRequest struct {
	TenantID   string     `json:"tenant_id,omitempty"`
	PathPrefix string     `json:"path_prefix"`
	Query      string     `json:"query"`
	Limit           int        `json:"limit" default:"10"`
	AsOf            *time.Time `json:"as_of"`
	SourceAgentID   string     `json:"source_agent_id"`
	EmbeddingSpace  string     `json:"embedding_space"`
}

type RetrieveResponse struct {
	Entries []MemoryEntry `json:"entries"`
	Total   int           `json:"total"`
}
