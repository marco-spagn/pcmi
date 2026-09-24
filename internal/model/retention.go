package model

import (
	"fmt"
	"strings"
	"time"
)

// Retention bounds (days) — mirror the CHECK constraints of migration 028.
const (
	RetentionMinDays = 1
	RetentionMaxDays = 36500
	eraseReasonMax   = 500
)

// RetentionPolicy is a per-tenant retention rule bound to an ltree path prefix
// (retention_policies, migration 028). PathPrefix "" applies tenant-wide.
type RetentionPolicy struct {
	ID                      int64     `json:"id"`
	TenantID                string    `json:"tenant_id"`
	PathPrefix              string    `json:"path_prefix"`
	SupersededRetentionDays *int      `json:"superseded_retention_days"`
	MaxAgeDays              *int      `json:"max_age_days"`
	Description             string    `json:"description"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// RetentionPolicyRequest is the body of PUT /v1/retention-policies (upsert by
// path_prefix). Omitted or null rules are cleared; at least one must be set.
type RetentionPolicyRequest struct {
	PathPrefix              string `json:"path_prefix"`
	SupersededRetentionDays *int   `json:"superseded_retention_days"`
	MaxAgeDays              *int   `json:"max_age_days"`
	Description             string `json:"description"`
}

// Normalize trims the prefix and validates the request.
func (r *RetentionPolicyRequest) Normalize() error {
	r.PathPrefix = strings.TrimSpace(r.PathPrefix)
	if err := ValidateRetentionPrefix(r.PathPrefix); err != nil {
		return err
	}
	if r.SupersededRetentionDays == nil && r.MaxAgeDays == nil {
		return fmt.Errorf("at least one of superseded_retention_days or max_age_days is required")
	}
	for name, v := range map[string]*int{
		"superseded_retention_days": r.SupersededRetentionDays,
		"max_age_days":              r.MaxAgeDays,
	} {
		if v != nil && (*v < RetentionMinDays || *v > RetentionMaxDays) {
			return fmt.Errorf("%s must be %d–%d (got %d)", name, RetentionMinDays, RetentionMaxDays, *v)
		}
	}
	if len(r.Description) > eraseReasonMax {
		return fmt.Errorf("description too long (max %d chars)", eraseReasonMax)
	}
	return nil
}

// ValidateRetentionPrefix accepts "" (tenant-wide) or a valid ltree path.
func ValidateRetentionPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if err := ValidateLtreePath(prefix); err != nil {
		return fmt.Errorf("invalid path_prefix: %w", err)
	}
	return nil
}

// EraseRequest is the body of POST /v1/memories/erase (GDPR right to erasure).
type EraseRequest struct {
	PathPrefix string `json:"path_prefix"`
	DryRun     bool   `json:"dry_run"`
	Reason     string `json:"reason"`
}

// Normalize trims and validates the request. An empty prefix is rejected so a
// request can never erase a whole tenant by accident.
func (r *EraseRequest) Normalize() error {
	r.PathPrefix = strings.TrimSpace(r.PathPrefix)
	r.Reason = strings.TrimSpace(r.Reason)
	if r.PathPrefix == "" {
		return fmt.Errorf("path_prefix is required")
	}
	if err := ValidateLtreePath(r.PathPrefix); err != nil {
		return fmt.Errorf("invalid path_prefix: %w", err)
	}
	if len(r.Reason) > eraseReasonMax {
		return fmt.Errorf("reason too long (max %d chars)", eraseReasonMax)
	}
	return nil
}

// EraseCounts reports how many rows an erasure removed (or would remove).
type EraseCounts struct {
	MemoryVersions    int64 `json:"memory_versions"`
	Paths             int64 `json:"paths"`
	Links             int64 `json:"links"`
	Distilled         int64 `json:"distilled"`
	LinkProposals     int64 `json:"link_proposals"`
	EntitySnapshots   int64 `json:"entity_snapshots"`
	AliasProposals    int64 `json:"alias_proposals"`
	ConsolidationRuns int64 `json:"consolidation_runs"`
}

// EraseResult is the response of POST /v1/memories/erase.
type EraseResult struct {
	PathPrefix    string      `json:"path_prefix"`
	DryRun        bool        `json:"dry_run"`
	Counts        EraseCounts `json:"counts"`
	GraphVertices int         `json:"graph_vertices"`
	GraphError    string      `json:"graph_error,omitempty"`
	ErasedAt      *time.Time  `json:"erased_at,omitempty"`
}
