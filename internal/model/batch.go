package model

import "time"

type BatchStoreRequest struct {
	Items []StoreRequest `json:"items"`
}

type BatchStoreResult struct {
	Results []BatchStoreItemResult `json:"results"`
	Total   int                    `json:"total"`
}

type BatchStoreItemResult struct {
	Index        int    `json:"index"`
	ID           int64  `json:"id,omitempty"`
	Status       string `json:"status"`
	Version      int    `json:"version,omitempty"`
	SupersededID *int64 `json:"superseded_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

type BatchRetrieveRequest struct {
	Queries []RetrieveRequest `json:"queries"`
}

type BatchRetrieveResponse struct {
	Results []RetrieveResponse `json:"results"`
	Total   int                `json:"total"`
}

type MemoryExportRequest struct {
	PathPrefix string `json:"path_prefix"`
	Limit      int    `json:"limit"`
	IncludeEmb bool   `json:"include_embeddings"`
}

type MemoryExportResponse struct {
	TenantID  string        `json:"tenant_id"`
	Exported  int           `json:"exported"`
	Entries   []MemoryEntry `json:"entries"`
	ExportedAt time.Time    `json:"exported_at"`
}

type MemoryImportRequest struct {
	Entries []StoreRequest `json:"entries"`
	Mode    string         `json:"mode"` // skip | overwrite
}

type MemoryImportResponse struct {
	Imported int                  `json:"imported"`
	Skipped  int                  `json:"skipped"`
	Results  []BatchStoreItemResult `json:"results"`
}

type TenantCreateRequest struct {
	Slug     string                 `json:"slug"`
	Name     string                 `json:"name"`
	Settings map[string]interface{} `json:"settings"`
}

type TenantResponse struct {
	ID        string                 `json:"id"`
	Slug      string                 `json:"slug"`
	Name      string                 `json:"name"`
	Settings  map[string]interface{} `json:"settings,omitempty"`
	CreatedAt time.Time              `json:"created_at,omitempty"`
}

type APIKeyRotateRequest struct {
	Name string `json:"name"`
}

type APIKeyRotateResponse struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id"`
	Name                 string     `json:"name"`
	Role                 string     `json:"role"`
	APIKey               string     `json:"api_key"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	PreviousKeyID        string     `json:"previous_key_id,omitempty"`
	RotationGraceEndsAt  *time.Time `json:"rotation_grace_ends_at,omitempty"`
}

type APIKeyCreateRequest struct {
	TenantID  string     `json:"tenant_id"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at"`
}
