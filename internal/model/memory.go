package model

import (
	"fmt"
	"time"
)

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
	Importance        float64    `json:"importance,omitempty"`
	AccessCount       int        `json:"access_count,omitempty"`
	LastAccessedAt    *time.Time `json:"last_accessed_at,omitempty"`
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
	// Importance in [0,1]; omitted or zero uses server default 0.5.
	Importance *float64 `json:"importance,omitempty"`
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

	// DecayEnabled applies temporal recency decay in hybrid scoring (default true).
	DecayEnabled *bool `json:"decay_enabled,omitempty"`
}

// UpdateImportanceRequest sets importance on the current version at path.
type UpdateImportanceRequest struct {
	Importance float64 `json:"importance"`
}

// NormalizeImportance clamps to [0,1]; nil uses defaultImportance.
func NormalizeImportance(v *float64) float64 {
	const defaultImportance = 0.5
	if v == nil {
		return defaultImportance
	}
	if *v < 0 {
		return 0
	}
	if *v > 1 {
		return 1
	}
	return *v
}

// ValidateImportance returns an error when importance is outside [0,1].
func ValidateImportance(v float64) error {
	if v < 0 || v > 1 {
		return fmt.Errorf("importance must be between 0 and 1")
	}
	return nil
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
