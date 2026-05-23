package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

func adminTestApp(t *testing.T, mock pgxmock.PgxPoolIface) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.RoleContextKey, "admin")
		c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		return c.Next()
	})
	SetupAdminRoutes(app, mock)
	return app
}

func TestAdminHandler_createAPIKey_invalidJSON_400(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	app := adminTestApp(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/api-keys", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestAdminHandler_createAPIKey_missingTenant_usesContext(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	rows := pgxmock.NewRows([]string{"id", "tenant_id", "name", "role"}).
		AddRow(uuid.New().String(), tenantID, "auto", "user")
	mock.ExpectQuery(`admin_create_api_key`).
		WithArgs(tenantID, pgxmock.AnyArg(), "auto", "user", pgxmock.AnyArg()).
		WillReturnRows(rows)

	app := adminTestApp(t, mock)
	body := `{"name":"auto"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
}

func TestAdminHandler_rotateAPIKey_notFound_404(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	keyID := uuid.New().String()
	mock.ExpectQuery(`admin_rotate_api_key`).
		WithArgs(keyID, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	app := adminTestApp(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/api-keys/"+keyID+"/rotate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
}

func TestAdminHandler_createTenant_invalidJSON_400(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	app := adminTestApp(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/tenants", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestAdminHandler_listAPIKeys_queryTenantOverride(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	override := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").String()
	mock.ExpectQuery(`FROM api_keys`).
		WithArgs(override, 50).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "name", "role", "is_active", "expires_at", "created_at", "last_used_at",
		}))

	app := adminTestApp(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/api-keys?tenant_id="+override, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 0 {
		t.Fatalf("total=%d", out.Total)
	}
}

func TestAdminHandler_requireAdminRole_403(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.RoleContextKey, "readonly")
		c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		return c.Next()
	})
	SetupAdminRoutes(app, mock)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/tenants", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d want 403", resp.StatusCode)
	}
}
