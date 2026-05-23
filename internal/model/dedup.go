package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// DedupMode controls ingest deduplication when content hashes match.
type DedupMode string

const (
	DedupModeNone  DedupMode = "none"
	DedupModeSkip  DedupMode = "skip"
	DedupModeLink  DedupMode = "link"
	DedupModeMerge DedupMode = "merge"
)

const dedupLinkType = "duplicate"

// ParseDedupMode normalizes and validates a dedup mode string.
func ParseDedupMode(s string) (DedupMode, error) {
	m := DedupMode(strings.ToLower(strings.TrimSpace(s)))
	switch m {
	case "", DedupModeNone, DedupModeSkip, DedupModeLink, DedupModeMerge:
		if m == "" {
			return DedupModeNone, nil
		}
		return m, nil
	default:
		return "", fmt.Errorf("invalid dedup_mode %q (want none|skip|link|merge)", s)
	}
}

// DedupLinkType is the memory_links.link_type used for cross-path dedup.
func DedupLinkType() string { return dedupLinkType }

// NormalizeContentForHash applies ingest normalization before hashing.
func NormalizeContentForHash(content string) string {
	s := strings.TrimSpace(content)
	s = strings.ToLower(s)
	return norm.NFC.String(s)
}

// ContentHash returns the SHA-256 hex digest of normalized content.
func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(NormalizeContentForHash(content)))
	return hex.EncodeToString(sum[:])
}
