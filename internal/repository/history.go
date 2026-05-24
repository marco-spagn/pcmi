package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

// ListPathHistory returns all versions for a path, newest version first.
func (r *MemoryRepository) ListPathHistory(
	ctx context.Context,
	tenantID, path string,
	page model.PageRequest,
) ([]model.MemoryEntry, model.PageResponse, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, model.PageResponse{}, fmt.Errorf("path is required")
	}
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	sortKey := model.SortKeyIDDesc

	q := `
		SELECT ` + memoryEntrySelectCols + `,
		       NULL::float8 AS relevance_score
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND path = $2::ltree`
	args := []any{tenantID, path}
	argN := 3
	clause, clauseArgs, err := KeysetIDClause(page.Cursor, sortKey, argN)
	if err != nil {
		return nil, model.PageResponse{}, err
	}
	q += clause
	args = append(args, clauseArgs...)
	argN += len(clauseArgs)
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT $%d`, argN)
	args = append(args, FetchLimit(limit))

	rows, err := r.r.Query(ctx, q, args...)
	if err != nil {
		return nil, model.PageResponse{}, fmt.Errorf("list path history: %w", err)
	}
	defer rows.Close()

	var entries []model.MemoryEntry
	for rows.Next() {
		e, scanErr := r.scanMemoryEntry(rows, true)
		if scanErr != nil {
			return nil, model.PageResponse{}, fmt.Errorf("history scan: %w", scanErr)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, model.PageResponse{}, err
	}
	return model.FinishInt64Page(entries, limit, sortKey,
		func(e model.MemoryEntry) int64 { return e.ID },
		func(e model.MemoryEntry) time.Time { return e.CreatedAt },
	)
}
