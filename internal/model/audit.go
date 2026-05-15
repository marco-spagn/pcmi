package model

import "time"

type AuditEntry struct {
	ID         int64     `json:"id"`
	TenantID   string    `json:"tenant_id"`
	APIKeyID   *string   `json:"api_key_id,omitempty"`
	EventType  string    `json:"event_type"`
	Path       string    `json:"path"`
	Method     string    `json:"method"`
	StatusCode int       `json:"status_code"`
	IPAddress  *string   `json:"ip_address,omitempty"`
	UserAgent  *string   `json:"user_agent,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type AuditListResponse struct {
	Entries []AuditEntry `json:"entries"`
	Total   int          `json:"total"`
	Limit   int          `json:"limit"`
	Offset  int          `json:"offset"`
}
