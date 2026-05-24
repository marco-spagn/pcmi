package model

import (
	"fmt"
	"regexp"
	"strings"
)

// ltreeLabelRe matches a single valid ltree label: ASCII letters, digits, underscore.
// PostgreSQL ltree labels allow Unicode letters too, but we enforce ASCII-only for
// predictable index behaviour and to avoid encoding surprises.
var ltreeLabelRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ValidateLtreePath returns an error if path is not a valid ltree label sequence.
// Rules:
//   - Non-empty, no leading/trailing dots.
//   - Each dot-delimited label matches [A-Za-z0-9_]+.
//   - Maximum 256 characters total (PostgreSQL ltree limit is 65535 bytes, but
//     we apply a conservative limit to prevent oversized index entries).
//
// This is called before any parameterised SQL query so that invalid paths are
// rejected with a clear 400 rather than a DB error that may leak schema info.
func ValidateLtreePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("ltree path must not be empty")
	}
	if len(path) > 256 {
		return fmt.Errorf("ltree path too long (%d chars, max 256)", len(path))
	}
	if strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return fmt.Errorf("ltree path must not start or end with '.'")
	}
	for _, label := range strings.Split(path, ".") {
		if label == "" {
			return fmt.Errorf("ltree path contains empty label (double dot)")
		}
		if !ltreeLabelRe.MatchString(label) {
			return fmt.Errorf("ltree label %q contains invalid characters (allowed: A-Za-z0-9_)", label)
		}
	}
	return nil
}
