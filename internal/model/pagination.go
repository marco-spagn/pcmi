package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// SortKeyIDDesc orders by primary key descending (after_id friendly).
	SortKeyIDDesc = "id_desc"
	// SortKeyCreatedAtDesc orders by created_at DESC (uuid-keyed tables).
	SortKeyCreatedAtDesc = "created_at_desc"
)

// PaginatedResponse is embedded in list endpoint JSON bodies.
type PaginatedResponse struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}

// CursorFromAfterID builds a first-class cursor from the legacy after_id query param.
func CursorFromAfterID(sortKey string, afterID int64) (Cursor, error) {
	if afterID < 1 {
		return Cursor{}, fmt.Errorf("after_id must be positive")
	}
	if strings.TrimSpace(sortKey) == "" {
		return Cursor{}, fmt.Errorf("sort key required")
	}
	return Cursor{
		Version: cursorVersion,
		SortKey: sortKey,
		LastID:  afterID,
	}, nil
}

// ParseAfterIDQuery parses ?after_id= from HTTP query strings.
func ParseAfterIDQuery(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid after_id")
	}
	return n, nil
}

// FinishInt64Page trims limit+1 rows and emits next_cursor / has_more.
func FinishInt64Page[T any](
	items []T,
	limit int,
	sortKey string,
	idOf func(T) int64,
	tsOf func(T) time.Time,
) ([]T, PageResponse, error) {
	if limit < 1 {
		limit = 1
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	if len(items) == 0 {
		return items, PageResponse{HasMore: false}, nil
	}
	last := items[len(items)-1]
	pageResp, err := MakeNextCursor(sortKey, idOf(last), tsOf(last), hasMore)
	if err != nil {
		return nil, PageResponse{}, err
	}
	return items, pageResp, nil
}
