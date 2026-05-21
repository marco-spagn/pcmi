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
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
}

type RetrieveRequest struct {
	TenantID   string     `json:"tenant_id,omitempty"`
	PathPrefix string     `json:"path_prefix"`
	Query      string     `json:"query"`
	Limit           int        `json:"limit" default:"10"`
	AsOf            *time.Time `json:"as_of"`
	SourceAgentID   string     `json:"source_agent_id"`
	EmbeddingSpace  string     `json:"embedding_space"`
	Tags            []string   `json:"tags,omitempty"`
	TagsMatch       string     `json:"tags_match"` // "any" (default) or "all"

	// Cursor (opaque, PR #5) — when set, the repository continues from the
	// keyset position encoded in the cursor and ignores any implicit "offset
	// 0". Empty cursor means "first page". Decode via model.DecodeCursor at
	// the handler boundary.
	Cursor string `json:"cursor,omitempty"`
}

type RetrieveResponse struct {
	Entries []MemoryEntry `json:"entries"`
	Total   int           `json:"total"`

	// NextCursor (PR #5) — opaque continuation token. Empty when the client
	// has reached the last page. Clients keep paginating while this is
	// non-empty; the wire format is intentionally stable across PCMI
	// versions (see model.Cursor.Version).
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more,omitempty"`
}
