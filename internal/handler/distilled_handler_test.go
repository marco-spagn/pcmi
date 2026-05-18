package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDistilledHandler_missingTenantContext(t *testing.T) {
	app := fiber.New()
	app.Get("/v1/distilled", NewDistilledHandler(nil).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled?path_prefix=root.p", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}

func TestDistilledHandler_missingPathPrefix(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	app.Get("/v1/distilled", NewDistilledHandler(nil).Get)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/distilled", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}
