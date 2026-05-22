package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func metricsTestApp(scrapeToken string) *fiber.App {
	app := fiber.New()
	app.Get("/metrics", MetricsScrapeAuth(scrapeToken), func(c *fiber.Ctx) error {
		return c.SendString("# prometheus\n")
	})
	return app
}

func TestMetricsEndpoint_NoToken_Accessible(t *testing.T) {
	app := metricsTestApp("")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
}

func TestMetricsEndpoint_WithToken_RequiresBearer(t *testing.T) {
	app := metricsTestApp("secret-scrape-token")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}

func TestMetricsEndpoint_WrongToken_Returns401(t *testing.T) {
	app := metricsTestApp("secret-scrape-token")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}

func TestMetricsEndpoint_CorrectToken_Returns200(t *testing.T) {
	app := metricsTestApp("secret-scrape-token")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret-scrape-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "# prometheus\n" {
		t.Fatalf("body %q", body)
	}
}
