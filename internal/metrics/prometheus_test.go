package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func TestMetricsEndpointExportsPCMI(t *testing.T) {
	app := fiber.New()
	app.Get("/metrics", func(c *fiber.Ctx) error {
		c.Set("Content-Type", `text/plain; version=0.0.4; charset=utf-8`)
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			return err
		}
		enc := expfmt.NewEncoder(c, expfmt.NewFormat(expfmt.TypeTextPlain))
		for _, mf := range mfs {
			if err := enc.Encode(mf); err != nil {
				return err
			}
		}
		return nil
	})

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
