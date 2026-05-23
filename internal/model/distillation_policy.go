package model

import "time"

// DistillationPolicy defines when automatic distillation should run for a path prefix.
type DistillationPolicy struct {
	ID              int64      `json:"id"`
	TenantID        string     `json:"tenant_id"`
	Name            string     `json:"name"`
	PathPrefix      string     `json:"path_prefix"`
	Enabled         bool       `json:"enabled"`
	CountThreshold  int        `json:"count_threshold"`
	MinIntervalSecs int        `json:"min_interval_secs"`
	MaxAgeSecs      *int       `json:"max_age_secs,omitempty"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateDistillationPolicyRequest is the body for POST /v1/distillation/policies.
type CreateDistillationPolicyRequest struct {
	Name            string `json:"name"`
	PathPrefix      string `json:"path_prefix"`
	Enabled         *bool  `json:"enabled"`
	CountThreshold  int    `json:"count_threshold"`
	MinIntervalSecs int    `json:"min_interval_secs"`
	MaxAgeSecs      *int   `json:"max_age_secs"`
}

// PatchDistillationPolicyRequest is the body for PATCH /v1/distillation/policies/{id}.
type PatchDistillationPolicyRequest struct {
	Name            *string `json:"name"`
	PathPrefix      *string `json:"path_prefix"`
	Enabled         *bool   `json:"enabled"`
	CountThreshold  *int    `json:"count_threshold"`
	MinIntervalSecs *int    `json:"min_interval_secs"`
	MaxAgeSecs      *int    `json:"max_age_secs"`
}

// DistillationRun records one policy-triggered distillation attempt.
type DistillationRun struct {
	ID           int64      `json:"id"`
	PolicyID     int64      `json:"policy_id"`
	TenantID     string     `json:"tenant_id"`
	PathPrefix   string     `json:"path_prefix"`
	Status       string     `json:"status"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	SourceCount  int        `json:"source_count"`
	DistilledID  *int64     `json:"distilled_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}
