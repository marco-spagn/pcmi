package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsEndpointExportsPCMI(t *testing.T) {
	IncStore()
	app := fiber.New()
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: false})))

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "pcmi_memory_stores_total") {
		t.Fatalf("missing pcmi metrics:\n%s", s[:min(500, len(s))])
	}
}

func TestMetricsEndpointAfterManyRequests(t *testing.T) {
	app := fiber.New()
	app.Use(Middleware())
	app.Get("/v1/health", func(c *fiber.Ctx) error { return c.SendStatus(200) })
	app.Post("/v1/memories", func(c *fiber.Ctx) error { return c.SendStatus(200) })
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: false})))

	for i := 0; i < 50; i++ {
		_, _ = app.Test(httptest.NewRequest("GET", "/v1/health", nil))
		_, _ = app.Test(httptest.NewRequest("POST", "/v1/memories", nil))
	}
	IncStore()

	resp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
