// Package model — cursor.go.
//
// Opaque, stable, signed cursor for keyset pagination across PCMI's list
// endpoints (memory list, history, events, audit, distilled).
//
// Why cursor-based pagination (PR #5):
//
//   - LIMIT/OFFSET produces inconsistent results when rows are inserted or
//     deleted between page fetches — the second page can repeat or skip
//     entries, and the database scans `offset` rows per call (O(offset)).
//   - Keyset pagination uses the last seen sort key (`id`, `created_at`,
//     ...) as the starting point of the next query, giving O(limit) cost
//     and stable ordering under concurrent writes.
//
// Wire format:
//
//	cursor := base64url(`{"v":1,"k":"<sortKey>","id":<lastID>,"ts":"<RFC3339Nano>"}`)
//
// The leading version byte (`v`) makes future schema bumps detectable.
// Cursors are NOT secrets and not strongly tamper-proof — they're opaque
// to clients, not authentication tokens. A malicious client can always
// re-craft an offset; the worst they can get is "rows for OUR tenant
// starting somewhere weird".
package model

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// cursorVersion is the on-wire schema version. Bump in lockstep with any
// breaking change to Cursor (additional sort keys, type changes, etc.).
const cursorVersion = 1

// Cursor represents the opaque continuation point for keyset pagination.
// Marshal it with EncodeCursor; the client passes the result back verbatim
// as `next_cursor` on the following request and DecodeCursor restores the
// fields.
type Cursor struct {
	// Version is the wire-format version (currently 1). DecodeCursor
	// rejects unknown versions so callers can roll forward safely.
	Version int `json:"v"`

	// SortKey identifies WHICH ordering the cursor was produced against.
	// Repository methods refuse to apply a cursor that doesn't match the
	// active sort, so a cursor from `ORDER BY id DESC` can't accidentally
	// drive a `ORDER BY created_at` query.
	SortKey string `json:"k"`

	// LastID is the last seen primary-key value. For most PCMI tables this
	// is the BIGINT `id`; for tables keyed by ltree path the repository
	// composes (Path, LastID) as the keyset tuple.
	LastID int64 `json:"id"`

	// LastTimestamp pairs with LastID when the ordering is time-based
	// (events, audit, history). UTC RFC3339Nano on wire.
	LastTimestamp time.Time `json:"ts,omitempty"`
}

// IsZero reports whether the cursor was never set (first page).
func (c Cursor) IsZero() bool {
	return c == Cursor{}
}

// EncodeCursor renders the cursor as a URL-safe base64 string. A zero
// Cursor returns the empty string — callers use that as "no continuation".
func EncodeCursor(c Cursor) (string, error) {
	if c.IsZero() {
		return "", nil
	}
	if c.Version == 0 {
		c.Version = cursorVersion
	}
	if c.Version != cursorVersion {
		return "", fmt.Errorf("cursor: unsupported version %d", c.Version)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("cursor encode: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor parses the opaque string produced by EncodeCursor. An empty
// string decodes to the zero Cursor (no error) — callers treat that as
// "start from the beginning". A malformed or wrong-version cursor returns
// an error so the handler can respond with 400, not 500.
func DecodeCursor(s string) (Cursor, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		// Some clients use standard base64 (with padding). Be forgiving.
		raw, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			return Cursor{}, fmt.Errorf("cursor decode: %w", err)
		}
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, fmt.Errorf("cursor unmarshal: %w", err)
	}
	if c.Version == 0 {
		// Legacy callers may emit no version field. Treat as v1.
		c.Version = cursorVersion
	}
	if c.Version != cursorVersion {
		return Cursor{}, fmt.Errorf("cursor: unsupported version %d (server understands %d)", c.Version, cursorVersion)
	}
	return c, nil
}

// PageRequest carries pagination input common to every paginated PCMI
// endpoint. Limit applies AFTER the cursor predicate, so passing
// Cursor + Limit=N returns "the next N rows after the cursor".
//
// Limit is clamped at the repository layer to a service-specific maximum
// (typically 200) — Limit=0 means "use the default".
type PageRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// PageResponse is the dual of PageRequest. NextCursor is empty when there
// are no more rows; clients should keep calling while NextCursor != "".
type PageResponse struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// MakeNextCursor is a convenience for repository methods: given the last
// row of the current page (and the sort key in use), it produces the
// PageResponse pair the handler returns to the client. Returns the
// zero-value PageResponse when no row is available (the caller is on the
// last page).
func MakeNextCursor(sortKey string, lastID int64, lastTimestamp time.Time, hasMore bool) (PageResponse, error) {
	if !hasMore {
		return PageResponse{HasMore: false}, nil
	}
	if sortKey == "" {
		return PageResponse{}, errors.New("MakeNextCursor: sortKey required")
	}
	enc, err := EncodeCursor(Cursor{
		Version:       cursorVersion,
		SortKey:       sortKey,
		LastID:        lastID,
		LastTimestamp: lastTimestamp,
	})
	if err != nil {
		return PageResponse{}, err
	}
	return PageResponse{
		NextCursor: enc,
		HasMore:    true,
	}, nil
}
