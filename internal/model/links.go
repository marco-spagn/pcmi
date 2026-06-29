package model

import (
	"fmt"
	"strings"
	"time"
)

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

// PublicLinkTypes are the link_type values a client may assign on
// POST /v1/memories/links. The "duplicate" link type (see DedupLinkType) is
// reserved for the dedup engine and is intentionally not user-assignable.
var PublicLinkTypes = []string{"causal", "temporal", "contradicts", "supports", "related"}

// DefaultLinkType is applied when a CreateLinkRequest omits link_type.
const DefaultLinkType = "related"

// NormalizeLinkType trims and lowercases a user-supplied link type, applies the
// default for empty input, and validates the result against PublicLinkTypes.
// It returns an error for any value outside the documented set.
func NormalizeLinkType(s string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return DefaultLinkType, nil
	}
	for _, v := range PublicLinkTypes {
		if t == v {
			return t, nil
		}
	}
	return "", fmt.Errorf("invalid link_type %q (want one of: %s)", s, strings.Join(PublicLinkTypes, ", "))
}

type StatsResponse struct {
	ActiveMemories    int `json:"active_memories"`
	SupersededMemories int `json:"superseded_memories"`
	DistilledCount    int `json:"distilled_count"`
	LinksCount        int `json:"links_count"`
	EventsCount       int `json:"events_count"`
	ExpiringSoon      int `json:"expiring_soon"`
}
