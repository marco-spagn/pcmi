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
	app := fiber.New()
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
