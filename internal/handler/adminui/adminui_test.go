package adminui

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestEmbeddedAdminHTMLMarkers(t *testing.T) {
	t.Helper()
	html := string(adminHTML)
	for _, needle := range []string{
		"PCMI Admin",
		"/v1/admin/tenants",
		"/v1/admin/api-keys",
		"/v1/ready",
		"id=\"tenants-tbody\"",
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("admin HTML missing %q", needle)
		}
	}
}

func TestRegisterServesAdminUI(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	Register(app)

	req := httptest.NewRequest(fiber.MethodGet, "/v1/admin/ui", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get(fiber.HeaderContentType)
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type %q, want text/html", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "PCMI Admin") {
		t.Fatal("response body missing dashboard title")
	}
}
