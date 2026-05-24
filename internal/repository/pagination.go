package repository

import (
	"fmt"

	"github.com/marco-spagn/pcmi/internal/model"
)

// KeysetTimeIDClause appends a keyset predicate for ORDER BY timeColumn DESC, id DESC.
func KeysetTimeIDClause(cur model.Cursor, sortKey, timeColumn string, argN int) (string, []any, error) {
	if cur.IsZero() {
		return "", nil, nil
	}
	if cur.SortKey != sortKey {
		return "", nil, fmt.Errorf("cursor sort mismatch: want %q got %q", sortKey, cur.SortKey)
	}
	if cur.LastID < 1 {
		return "", nil, fmt.Errorf("invalid cursor id")
	}
	if cur.LastTimestamp.IsZero() {
		return fmt.Sprintf(" AND id < $%d", argN), []any{cur.LastID}, nil
	}
	return fmt.Sprintf(" AND (%s < $%d OR (%s = $%d AND id < $%d))", timeColumn, argN, timeColumn, argN, argN+1),
		[]any{cur.LastTimestamp, cur.LastID}, nil
}

// KeysetIDClause paginates tables ordered by id DESC.
func KeysetIDClause(cur model.Cursor, sortKey string, argN int) (string, []any, error) {
	if cur.IsZero() {
		return "", nil, nil
	}
	if cur.SortKey != sortKey {
		return "", nil, fmt.Errorf("cursor sort mismatch: want %q got %q", sortKey, cur.SortKey)
	}
	if cur.LastID < 1 {
		return "", nil, fmt.Errorf("invalid cursor id")
	}
	return fmt.Sprintf(" AND id < $%d", argN), []any{cur.LastID}, nil
}

// KeysetCreatedAtIDClause is an alias for audit/links tables using created_at.
func KeysetCreatedAtIDClause(cur model.Cursor, sortKey string, argN int) (string, []any, error) {
	return KeysetTimeIDClause(cur, sortKey, "created_at", argN)
}

// KeysetTimeClause paginates uuid-keyed tables by timestamp only.
func KeysetTimeClause(cur model.Cursor, sortKey, timeColumn string, argN int) (string, []any, error) {
	if cur.IsZero() {
		return "", nil, nil
	}
	if cur.SortKey != sortKey {
		return "", nil, fmt.Errorf("cursor sort mismatch: want %q got %q", sortKey, cur.SortKey)
	}
	if cur.LastTimestamp.IsZero() {
		return "", nil, fmt.Errorf("invalid cursor timestamp")
	}
	return fmt.Sprintf(" AND %s < $%d", timeColumn, argN), []any{cur.LastTimestamp}, nil
}

// FetchLimit returns limit+1 for has_more detection.
func FetchLimit(limit int) int {
	if limit < 1 {
		return 2
	}
	return limit + 1
}
