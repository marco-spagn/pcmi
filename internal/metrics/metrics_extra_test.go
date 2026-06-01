package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestIncGraphTraversalMetric(t *testing.T) {
	IncGraphTraversal()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: false}))

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	s := string(body)
	if !strings.Contains(s, "pcmi_graph_traversal_total") {
		t.Fatalf("missing graph traversal counter:\n%s", s[:min(500, len(s))])
	}
}

func TestObserveGraphTraversalMetric(t *testing.T) {
	ObserveGraphTraversal(1.5)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: false}))

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	s := string(body)
	if !strings.Contains(s, "pcmi_graph_traversal_duration_seconds") {
		t.Fatalf("missing graph traversal duration histogram:\n%s", s[:min(500, len(s))])
	}
}
