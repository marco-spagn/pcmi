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
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

var errNoRows = errors.New("no rows")

func TestDistillationPolicyHandler_Create_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at",
	}).AddRow(int64(1), tenantID, "test-policy", "root.test", true, 10, 300, nil, nil, now, now)

	mock.ExpectQuery(`INSERT INTO distillation_policies`).
		WithArgs(tenantID, "test-policy", "root.test", true, 10, 300, pgxmock.AnyArg()).
		WillReturnRows(rows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Post("/v1/distillation/policies", h.Create)

	req := httptest.NewRequest("POST", "/v1/distillation/policies",
		strings.NewReader(`{"name":"test-policy","path_prefix":"root.test","count_threshold":10,"min_interval_secs":300,"max_age_secs":null}`))
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

	var created map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created["name"] != "test-policy" {
		t.Fatalf("name=%v", created["name"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDistillationPolicyHandler_Create_SetTenantError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnError(errNoRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Post("/v1/distillation/policies", h.Create)

	req := httptest.NewRequest("POST", "/v1/distillation/policies",
		strings.NewReader(`{"name":"p","path_prefix":"root.x"}`))
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

func TestDistillationPolicyHandler_Create_InsertError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`INSERT INTO distillation_policies`).
		WithArgs(tenantID, "p", "root.x", true, 10, 300, pgxmock.AnyArg()).
		WillReturnError(errNoRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Post("/v1/distillation/policies", h.Create)

	req := httptest.NewRequest("POST", "/v1/distillation/policies",
		strings.NewReader(`{"name":"p","path_prefix":"root.x"}`))
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

func TestDistillationPolicyHandler_Create_CountThresholdZero(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at",
	}).AddRow(int64(2), tenantID, "p", "root.y", true, 10, 300, nil, nil, now, now)

	// count_threshold defaults to 10 when < 1
	mock.ExpectQuery(`INSERT INTO distillation_policies`).
		WithArgs(tenantID, "p", "root.y", true, 10, 0, pgxmock.AnyArg()).
		WillReturnRows(rows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Post("/v1/distillation/policies", h.Create)

	req := httptest.NewRequest("POST", "/v1/distillation/policies",
		strings.NewReader(`{"name":"p","path_prefix":"root.y","count_threshold":0,"min_interval_secs":0}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
}

func TestDistillationPolicyHandler_List_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	policyRows := pgxmock.NewRows([]string{
		"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at",
	}).AddRow(int64(1), tenantID, "pol1", "root.a", true, 5, 60, nil, nil, now, now)

	mock.ExpectQuery(`SELECT id, tenant_id::text, name`).
		WithArgs(tenantID, 51).
		WillReturnRows(policyRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Get("/v1/distillation/policies", h.List)

	req := httptest.NewRequest("GET", "/v1/distillation/policies?limit=50", nil)
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

func TestDistillationPolicyHandler_Patch_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	// Patch with empty body → name="" and path_prefix="" (strings), all nil pointers
	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at",
	}).AddRow(int64(1), tenantID, "old-name", "root.old", true, 10, 300, nil, nil, now, now)

	mock.ExpectQuery(`UPDATE distillation_policies SET`).
		WithArgs(int64(1), tenantID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(rows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Patch("/v1/distillation/policies/:id", h.Patch)

	req := httptest.NewRequest("PATCH", "/v1/distillation/policies/1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
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

func TestDistillationPolicyHandler_Patch_NotFound(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`UPDATE distillation_policies SET`).
		WithArgs(int64(99), tenantID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Patch("/v1/distillation/policies/:id", h.Patch)

	req := httptest.NewRequest("PATCH", "/v1/distillation/policies/99", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestDistillationPolicyHandler_ListRuns_WithMock(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "22222222-2222-2222-2222-222222222222"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	runRows := pgxmock.NewRows([]string{
		"id", "policy_id", "tenant_id", "path_prefix", "status", "error_message",
		"source_count", "distilled_id", "created_at", "completed_at",
	}).AddRow(int64(1), int64(1), tenantID, "root.a", "completed", nil, 10, nil, now, nil)

	mock.ExpectQuery(`SELECT id, policy_id, tenant_id::text, path_prefix`).
		WithArgs(tenantID, 51).
		WillReturnRows(runRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Get("/v1/distillation/runs", h.ListRuns)

	req := httptest.NewRequest("GET", "/v1/distillation/runs?limit=50", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDistillationPolicyHandler_ListRuns_WithPolicyID(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "33333333-3333-3333-3333-333333333333"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	runRows := pgxmock.NewRows([]string{
		"id", "policy_id", "tenant_id", "path_prefix", "status", "error_message",
		"source_count", "distilled_id", "created_at", "completed_at",
	}).AddRow(int64(5), int64(42), tenantID, "root.b", "pending", nil, 3, nil, now, nil)

	mock.ExpectQuery(`SELECT id, policy_id, tenant_id::text, path_prefix`).
		WithArgs(tenantID, int64(42), 51).
		WillReturnRows(runRows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Get("/v1/distillation/runs", h.ListRuns)

	req := httptest.NewRequest("GET", "/v1/distillation/runs?limit=50&policy_id=42", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDistillationPolicyHandler_Create_EnabledFalse(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "44444444-4444-4444-4444-444444444444"
	now := time.Now().UTC()

	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "name", "path_prefix", "enabled",
		"count_threshold", "min_interval_secs", "max_age_secs", "last_triggered_at",
		"created_at", "updated_at",
	}).AddRow(int64(3), tenantID, "disabled-pol", "root.z", false, 10, 300, nil, nil, now, now)

	// enabled=false in request, MinIntervalSecs defaults to 0
	mock.ExpectQuery(`INSERT INTO distillation_policies`).
		WithArgs(tenantID, "disabled-pol", "root.z", false, 10, 0, pgxmock.AnyArg()).
		WillReturnRows(rows)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		return c.Next()
	})

	h := NewDistillationPolicyHandler(mock)
	app.Post("/v1/distillation/policies", h.Create)

	req := httptest.NewRequest("POST", "/v1/distillation/policies",
		strings.NewReader(`{"name":"disabled-pol","path_prefix":"root.z","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
}

func TestDistillationPolicyHandler_SetupRoutes(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	SetupDistillationPolicyRoutes(app, mock)

	// Verify that the routes are registered by checking that
	// the middleware-protected endpoints reject unauthenticated requests
	req := httptest.NewRequest("POST", "/v1/distillation/policies",
		strings.NewReader(`{"name":"x","path_prefix":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// Without tenant context, the handler should return 401
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}
