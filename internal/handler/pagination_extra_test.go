package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestPaginatedJSON(t *testing.T) {
	t.Parallel()

	page := model.PageResponse{
		NextCursor: "abc123",
		HasMore:    true,
	}
	result := PaginatedJSON([]string{"a", "b"}, 10, page)

	if result["entries"] == nil {
		t.Fatal("entries missing")
	}
	if result["limit"] != 10 {
		t.Fatalf("limit=%v", result["limit"])
	}
	if result["next_cursor"] != "abc123" {
		t.Fatalf("next_cursor=%v", result["next_cursor"])
	}
	if result["has_more"] != true {
		t.Fatalf("has_more=%v", result["has_more"])
	}
}

func TestPaginatedJSON_NoMore(t *testing.T) {
	t.Parallel()

	page := model.PageResponse{NextCursor: "", HasMore: false}
	result := PaginatedJSON(nil, 50, page)

	if result["has_more"] != false {
		t.Fatal("expected has_more=false")
	}
	if result["next_cursor"] != "" {
		t.Fatalf("expected empty next_cursor, got %v", result["next_cursor"])
	}
}

func TestParseListPagination_DefaultZeroLimit(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	var got ListPageParams
	var parseErr error
	app.Get("/", func(c *fiber.Ctx) error {
		got, parseErr = ParseListPagination(c, model.SortKeyIDDesc, 0)
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if got.Limit != 50 {
		t.Fatalf("expected default limit 50, got %d", got.Limit)
	}
}

func TestParseListPagination_InvalidAfterID(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	var parseErr error
	app.Get("/", func(c *fiber.Ctx) error {
		_, parseErr = ParseListPagination(c, model.SortKeyIDDesc, 50)
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/?after_id=not-a-number", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if parseErr == nil {
		t.Fatal("expected error for invalid after_id")
	}
}

func TestParseListPagination_CursorSortMismatch(t *testing.T) {
	t.Parallel()

	// Create a valid cursor with one sort key, then use a different expected sort key
	validCursor, err := model.EncodeCursor(model.Cursor{LastID: 42, SortKey: model.SortKeyIDDesc})
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	var parseErr error
	app.Get("/", func(c *fiber.Ctx) error {
		_, parseErr = ParseListPagination(c, model.SortKeyCreatedAtDesc, 50)
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/?cursor="+validCursor, nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if parseErr == nil {
		t.Fatal("expected sort mismatch error")
	}
}

func TestDefaultListLimit(t *testing.T) {
	t.Parallel()

	if defaultListLimit != 50 {
		t.Fatalf("defaultListLimit=%d", defaultListLimit)
	}
	if maxListLimit != 200 {
		t.Fatalf("maxListLimit=%d", maxListLimit)
	}
}
