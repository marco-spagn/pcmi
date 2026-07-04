package model

import "time"

const (
	EntityAliasSourceManual        = "manual"
	EntityAliasSourceExtraction    = "extraction"
	EntityAliasSourceAliasProposal = "alias_proposal"

	EntityAliasProposalStatusPending  = "pending"
	EntityAliasProposalStatusAccepted = "accepted"
	EntityAliasProposalStatusRejected = "rejected"
)

// EntityRegistry is a canonical entity identity for a tenant + kind.
type EntityRegistry struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	CanonicalKey string         `json:"canonical_key"`
	DisplayName  string         `json:"display_name,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// EntityAlias maps a normalized alias string to a canonical entity.
type EntityAlias struct {
	ID         string         `json:"id"`
	EntityID   string         `json:"entity_id"`
	Kind       string         `json:"kind"`
	AliasKey   string         `json:"alias_key"`
	Source     string         `json:"source"`
	Confidence float64        `json:"confidence"`
	ValidFrom  time.Time      `json:"valid_from"`
	ValidTo    *time.Time     `json:"valid_to,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// EntitySnapshot records entity state observed from one memory extraction version.
type EntitySnapshot struct {
	ID            int64          `json:"id"`
	EntityID      string         `json:"entity_id"`
	MemoryID      int64          `json:"memory_id"`
	MemoryVersion int            `json:"memory_version"`
	ProfileID     string         `json:"profile_id,omitempty"`
	Slot          string         `json:"slot"`
	RawKey        string         `json:"raw_key,omitempty"`
	Attributes    map[string]any `json:"attributes,omitempty"`
	Confidence    float64        `json:"confidence"`
	CreatedAt     time.Time      `json:"created_at"`
}

// EntityAliasProposal queues LLM-suggested entity merges awaiting review.
type EntityAliasProposal struct {
	ID              int64          `json:"id"`
	Kind            string         `json:"kind"`
	AliasKey        string         `json:"alias_key"`
	SourceEntityID  string         `json:"source_entity_id"`
	TargetEntityID  string         `json:"target_entity_id"`
	SourceMemoryID  int64          `json:"source_memory_id,omitempty"`
	Status          string         `json:"status"`
	Confidence      float64        `json:"confidence"`
	Reason          string         `json:"reason"`
	Model           string         `json:"model,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	ReviewedAt      *time.Time     `json:"reviewed_at,omitempty"`
}
