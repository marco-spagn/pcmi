package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

// TestAPIKeyMiddleware_SetTenantContextFailureReturns503 verifies BUG-FIX-5:
// set_tenant_context failure must return 503 and must not invoke downstream handlers.
func TestAPIKeyMiddleware_SetTenantContextFailureReturns503(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	raw := "pcmi_tenant_ctx_fail"
	hash := apiKeyHash(raw)
	keyID := uuid.New().String()
	tenantID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").String()

	rows := pgxmock.NewRows([]string{"id", "tenant_id", "role", "is_active"}).
		AddRow(keyID, tenantID, "user", true)
	mock.ExpectQuery(`FROM api_keys`).WithArgs(hash).WillReturnRows(rows)
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).
		WillReturnError(errors.New("set_tenant_context unavailable"))

	app := fiber.New()
	hit := false
	app.Use(apiKeyMiddleware(mock))
	app.Get("/v1/stats", func(c *fiber.Ctx) error {
		hit = true
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("X-API-Key", raw)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s, want 503", resp.StatusCode, b)
	}
	if hit {
		t.Fatal("downstream handler ran after set_tenant_context failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
