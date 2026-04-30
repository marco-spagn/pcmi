package model

import "time"

type MemoryEntry struct {
	ID             int64      `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Path           string     `json:"path"`
	Content        string     `json:"content"`
	Metadata       any        `json:"metadata"`
	Tags           []string   `json:"tags"`
	Embedding      []float32  `json:"embedding"`
	EmbeddingModel string     `json:"embedding_model"`
	Version        int        `json:"version"`
	ValidFrom      time.Time  `json:"valid_from"`
	ValidTo        *time.Time `json:"valid_to"`
	SourceAgentID  *string    `json:"source_agent_id"`
	SourceEventID  string     `json:"source_event_id"`
	CreatedAt      time.Time  `json:"created_at"`
}

type StoreRequest struct {
	TenantID       string                 `json:"tenant_id" validate:"required"`
	Path           string                 `json:"path" validate:"required"`
	Content        string                 `json:"content" validate:"required"`
	Metadata       map[string]interface{} `json:"metadata"`
	Tags           []string               `json:"tags"`
	Embedding      []float32              `json:"embedding"`
	EmbeddingModel string                 `json:"embedding_model"`
	SourceAgentID  string                 `json:"source_agent_id"`
}

type RetrieveRequest struct {
	TenantID   string     `json:"tenant_id" validate:"required"`
	PathPrefix string     `json:"path_prefix"`
	Query      string     `json:"query"`
	Limit      int        `json:"limit" default:"10"`
	AsOf       *time.Time `json:"as_of"`
}

type RetrieveResponse struct {
	Entries []MemoryEntry `json:"entries"`
	Total   int           `json:"total"`
}
