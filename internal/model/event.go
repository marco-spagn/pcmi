package model

import "time"

// IngestEventRequest is the universal event ingestion body (POST /v1/events).
type IngestEventRequest struct {
	EventType     string                 `json:"event_type"`
	AgentID       string                 `json:"agent_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Payload       map[string]interface{} `json:"payload"`
}

// IngestEventResponse is returned after persisting an ingested event.
type IngestEventResponse struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
}

// HistoryResponse lists all versions for a path (GET /v1/memories/history).
type HistoryResponse struct {
	Path    string        `json:"path"`
	Entries []MemoryEntry `json:"entries"`
	Total   int           `json:"total"`
}
