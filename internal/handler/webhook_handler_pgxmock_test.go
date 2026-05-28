package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

func TestWebhookHandler_Register_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	rows := pgxmock.NewRows([]string{"id"}).AddRow("wh-123")
	mock.ExpectQuery(`INSERT INTO webhook_endpoints`).
		WithArgs(tenantID, "https://example.com/hook", []string{"memory.stored"}, "").
		WillReturnRows(rows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewWebhookHandler(mock)
	app.Post("/v1/webhooks", h.Register)

	req := httptest.NewRequest("POST", "/v1/webhooks",
		strings.NewReader(`{"url":"https://example.com/hook","event_types":["memory.stored"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 201, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["id"] != "wh-123" {
		t.Fatalf("id=%v", result["id"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWebhookHandler_Register_DBError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnError(errors.New("db error"))

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewWebhookHandler(mock)
	app.Post("/v1/webhooks", h.Register)

	req := httptest.NewRequest("POST", "/v1/webhooks",
		strings.NewReader(`{"url":"https://example.com/hook"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_List_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	whRows := pgxmock.NewRows([]string{"id", "url", "event_types", "enabled", "created_at"}).
		AddRow("wh-1", "https://hooks.example.com/1", []string{"memory.stored"}, true, now).
		AddRow("wh-2", "https://hooks.example.com/2", []string{}, false, now)

	mock.ExpectQuery(`SELECT id::text, url, event_types`).
		WithArgs(tenantID, 51).
		WillReturnRows(whRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewWebhookHandler(mock)
	app.Get("/v1/webhooks", h.List)

	req := httptest.NewRequest("GET", "/v1/webhooks?limit=50", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWebhookHandler_DeadLetter_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	dlRows := pgxmock.NewRows([]string{
		"id", "endpoint_id", "event_type", "payload", "attempts", "last_error", "created_at",
	}).AddRow("dl-1", "ep-1", "memory.stored", map[string]any{"k": "v"}, 5, "timeout", now)

	mock.ExpectQuery(`SELECT wd.id::text, wd.endpoint_id::text, wd.event_type`).
		WithArgs(tenantID, 51).
		WillReturnRows(dlRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewWebhookHandler(mock)
	app.Get("/v1/webhooks/dead-letter", h.DeadLetter)

	req := httptest.NewRequest("GET", "/v1/webhooks/dead-letter?limit=50", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWebhookHandler_DeadLetter_DBError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnError(errors.New("db error"))

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewWebhookHandler(mock)
	app.Get("/v1/webhooks/dead-letter", h.DeadLetter)

	req := httptest.NewRequest("GET", "/v1/webhooks/dead-letter?limit=50", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}
