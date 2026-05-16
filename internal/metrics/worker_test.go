package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestWorkerMetricsExportsPCMI(t *testing.T) {
	IncWorkerRedisEvent("memory.stored")
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(WorkerRegistry, promhttp.HandlerOpts{EnableOpenMetrics: false}))

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	s := string(body)
	if !strings.Contains(s, "pcmi_worker_redis_events_total") {
		t.Fatalf("missing worker metrics:\n%s", s[:min(600, len(s))])
	}
}
