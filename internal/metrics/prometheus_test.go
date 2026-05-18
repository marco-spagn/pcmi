package metrics

import (
	"io"
	"net/http"
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

func TestMetricsMiddlewarePassesThrough(t *testing.T) {
	app := fiber.New()
	app.Use(Middleware())
	var reached bool
	app.Get("/", func(c *fiber.Ctx) error {
		reached = true
		return c.SendString("x")
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !reached {
		t.Fatal("handler not reached")
	}
}

func TestIncRetrieveMetric(t *testing.T) {
	IncRetrieve()
	app := fiber.New()
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: false})))

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "pcmi_memory_retrieves_total") {
		t.Fatalf("missing retrieves counter")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
