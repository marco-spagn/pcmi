package model

import (
	"strconv"
	"time"
)

const (
	LinkProposalStatusPending  = "pending"
	LinkProposalStatusAccepted = "accepted"
	LinkProposalStatusRejected = "rejected"
)

// LinkProposal is a queued LLM-suggested memory_links edge awaiting review.
type LinkProposal struct {
	ID              int64      `json:"id"`
	SourceMemoryID  int64      `json:"source_memory_id"`
	FromMemoryID    int64      `json:"from_memory_id"`
	ToMemoryID      int64      `json:"to_memory_id"`
	FromPath        string     `json:"from_path"`
	ToPath          string     `json:"to_path"`
	LinkType        string     `json:"link_type"`
	Status          string     `json:"status"`
	Confidence      float64    `json:"confidence"`
	Reason          string     `json:"reason"`
	ProfileID       string     `json:"profile_id,omitempty"`
	Model           string     `json:"model,omitempty"`
	Metadata        any        `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
}

// MemoryPathForID returns the canonical ltree path used in memory_links.
func MemoryPathForID(id int64) string {
	return "memory." + strconv.FormatInt(id, 10)
}
