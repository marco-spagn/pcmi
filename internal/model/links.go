package model

import "time"

type MemoryLink struct {
	ID        int64      `json:"id"`
	FromPath  string     `json:"from_path"`
	ToPath    string     `json:"to_path"`
	LinkType  string     `json:"link_type"`
	Metadata  any        `json:"metadata"`
	CreatedAt time.Time  `json:"created_at"`
}

type CreateLinkRequest struct {
	FromPath string                 `json:"from_path"`
	ToPath   string                 `json:"to_path"`
	LinkType string                 `json:"link_type"`
	Metadata map[string]interface{} `json:"metadata"`
}

type StatsResponse struct {
	ActiveMemories    int `json:"active_memories"`
	SupersededMemories int `json:"superseded_memories"`
	DistilledCount    int `json:"distilled_count"`
	LinksCount        int `json:"links_count"`
	EventsCount       int `json:"events_count"`
	ExpiringSoon      int `json:"expiring_soon"`
}
