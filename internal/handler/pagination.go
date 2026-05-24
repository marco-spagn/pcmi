package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/model"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// ListPageParams holds normalized cursor pagination input from query params.
type ListPageParams struct {
	Limit  int
	Cursor model.Cursor
}

// ParseListPagination reads limit, cursor, and after_id from a Fiber query string.
// after_id is a convenience alias that builds a cursor when cursor is absent.
func ParseListPagination(c *fiber.Ctx, sortKey string, defaultLimit int) (ListPageParams, error) {
	if defaultLimit <= 0 {
		defaultLimit = defaultListLimit
	}
	limit := c.QueryInt("limit", defaultLimit)
	if limit < 1 {
		limit = 1
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	var cur model.Cursor
	cursorRaw := strings.TrimSpace(c.Query("cursor"))
	afterRaw := strings.TrimSpace(c.Query("after_id"))

	if cursorRaw != "" && afterRaw != "" {
		return ListPageParams{}, fmt.Errorf("use either cursor or after_id, not both")
	}

	switch {
	case cursorRaw != "":
		decoded, err := model.DecodeCursor(cursorRaw)
		if err != nil {
			return ListPageParams{}, fmt.Errorf("invalid cursor: %w", err)
		}
		if !decoded.IsZero() && decoded.SortKey != sortKey {
			return ListPageParams{}, fmt.Errorf("cursor sort mismatch")
		}
		cur = decoded
	case afterRaw != "":
		afterID, err := model.ParseAfterIDQuery(afterRaw)
		if err != nil {
			return ListPageParams{}, err
		}
		built, err := model.CursorFromAfterID(sortKey, afterID)
		if err != nil {
			return ListPageParams{}, err
		}
		cur = built
	}

	return ListPageParams{Limit: limit, Cursor: cur}, nil
}

// PaginatedJSON merges entries with standard pagination response fields.
func PaginatedJSON(entries any, limit int, page model.PageResponse) fiber.Map {
	return fiber.Map{
		"entries":     entries,
		"limit":       limit,
		"next_cursor": page.NextCursor,
		"has_more":    page.HasMore,
	}
}
