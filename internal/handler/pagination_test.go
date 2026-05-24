package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestPagination_ParseListPagination_DefaultLimit(t *testing.T) {
	app := fiber.New()
	var got ListPageParams
	var parseErr error
	app.Get("/", func(c *fiber.Ctx) error {
		got, parseErr = ParseListPagination(c, model.SortKeyIDDesc, 50)
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/?limit=25", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if got.Limit != 25 || !got.Cursor.IsZero() {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestPagination_ParseListPagination_AfterID(t *testing.T) {
	app := fiber.New()
	var got ListPageParams
	var parseErr error
	app.Get("/", func(c *fiber.Ctx) error {
		got, parseErr = ParseListPagination(c, model.SortKeyIDDesc, 50)
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/?after_id=99&limit=10", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if got.Cursor.LastID != 99 || got.Cursor.SortKey != model.SortKeyIDDesc {
		t.Fatalf("unexpected cursor %+v", got.Cursor)
	}
}

func TestPagination_ParseListPagination_InvalidCursor(t *testing.T) {
	app := fiber.New()
	var parseErr error
	app.Get("/", func(c *fiber.Ctx) error {
		_, parseErr = ParseListPagination(c, model.SortKeyIDDesc, 50)
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/?cursor=not-valid", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if parseErr == nil {
		t.Fatal("expected invalid cursor error")
	}
}
