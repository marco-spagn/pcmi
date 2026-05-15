package middleware

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequireWriteRoleReadonlyBlocked(t *testing.T) {
	app := fiber.New()
	app.Post("/write", func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "readonly")
		return RequireWriteRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(201)
	})

	req := httptest.NewRequest("POST", "/write", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRequireWriteRoleUserAllowed(t *testing.T) {
	app := fiber.New()
	app.Post("/write", func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "user")
		return RequireWriteRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(201)
	})

	req := httptest.NewRequest("POST", "/write", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d body=%s", resp.StatusCode, body)
	}
}
