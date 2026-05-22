package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestMetricsEndpoint_BearerTokenTrimmed(t *testing.T) {
	app := metricsTestApp("secret-scrape-token")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer   secret-scrape-token  ")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestMetricsEndpoint_BearerPrefixOnly_Returns401(t *testing.T) {
	app := metricsTestApp("secret")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer ")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}

func TestMetricsEndpoint_RawTokenWithoutBearer_Returns401(t *testing.T) {
	app := metricsTestApp("secret")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}

func TestIsUnauthenticatedProbe_onlyGET(t *testing.T) {
	if IsUnauthenticatedProbe(fiber.MethodPost, "/health") {
		t.Fatal("POST /health must not be a probe")
	}
	if !IsUnauthenticatedProbe(fiber.MethodGet, "/v1/ready") {
		t.Fatal("GET /v1/ready must be a probe")
	}
}
