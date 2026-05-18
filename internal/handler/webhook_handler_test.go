package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestWebhookRegisterMissingTenantContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/hook", NewWebhookHandler(nil).Register)

	req := httptest.NewRequest("POST", "/hook", strings.NewReader(`{"url":"https://example.com/h"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebhookListMissingTenantContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/hooks", NewWebhookHandler(nil).List)

	resp, err := app.Test(httptest.NewRequest("GET", "/hooks", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebhookDeadLetterMissingTenantContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/dlq", NewWebhookHandler(nil).DeadLetter)

	resp, err := app.Test(httptest.NewRequest("GET", "/dlq", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}
