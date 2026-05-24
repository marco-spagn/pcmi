package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func apiKeyHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func TestAPIKeyMiddleware_validKey_setsContext(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	raw := "pcmi_testkey"
	hash := apiKeyHash(raw)
	keyID := uuid.New().String()
	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()

	rows := pgxmock.NewRows([]string{"id", "tenant_id", "role", "is_active"}).
		AddRow(keyID, tenantID, "user", true)
	mock.ExpectQuery(`FROM api_keys`).WithArgs(hash).WillReturnRows(rows)
	mock.ExpectExec(`set_tenant_context`).WithArgs(tenantID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(`UPDATE api_keys SET last_used_at`).WithArgs(keyID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	app := fiber.New()
	app.Use(apiKeyMiddleware(mock))
	app.Get("/v1/stats", func(c *fiber.Ctx) error {
		if c.Locals(TenantContextKey) != tenantID {
			t.Fatalf("tenant=%v", c.Locals(TenantContextKey))
		}
		if c.Locals(RoleContextKey) != "user" {
			t.Fatalf("role=%v", c.Locals(RoleContextKey))
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("X-API-Key", raw)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := mock.ExpectationsWereMet(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expectations: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAPIKeyMiddleware_inactiveKey_401(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	hash := apiKeyHash("pcmi_inactive")
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "role", "is_active"}).
		AddRow(uuid.New().String(), uuid.New().String(), "user", false)
	mock.ExpectQuery(`FROM api_keys`).WithArgs(hash).WillReturnRows(rows)

	app := fiber.New()
	app.Use(apiKeyMiddleware(mock))
	app.Get("/v1/stats", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("X-API-Key", "pcmi_inactive")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestAPIKeyMiddleware_unknownHash_401(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`FROM api_keys`).WithArgs(pgxmock.AnyArg()).WillReturnError(pgx.ErrNoRows)

	app := fiber.New()
	app.Use(apiKeyMiddleware(mock))
	app.Get("/v1/stats", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("X-API-Key", "pcmi_unknown")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestAPIKeyMiddleware_whitespaceKey_401(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(apiKeyMiddleware(nil))
	app.Get("/v1/stats", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("X-API-Key", "   ")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("whitespace key should be rejected, status=%d", resp.StatusCode)
	}
}

func TestAPIKeyMiddleware_clientIP_ignoresSpoofedXFFWithoutTrustProxy(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/ip", func(c *fiber.Ctx) error {
		return c.SendString(c.IP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.RemoteAddr = "127.0.0.1:12345"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "203.0.113.50" {
		t.Fatal("X-Forwarded-For must not be trusted when TrustProxy is disabled")
	}
}
