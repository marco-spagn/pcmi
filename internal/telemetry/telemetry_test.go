package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"go.opentelemetry.io/otel"
)

func TestInitNoExporter(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	shutdown, err := Init(context.Background(), config.Load(), "pcmi-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInit_prefersOTELServiceNameWithoutExporter(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "custom-svc")
	cfg := config.Load()
	shutdown, err := Init(context.Background(), cfg, "ignored-when-env-set")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInit_emptyServiceNameUsesDefault(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	// No OTLP endpoint → noop provider; exercise default branch for service name resolution.
	shutdown, err := Init(context.Background(), config.Load(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInit_OTLPHTTPExporterPostsTraces(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", srv.URL+"/v1/traces")
	t.Setenv("OTEL_SERVICE_NAME", "otel-httptest")

	ctx := context.Background()
	shutdown, err := Init(ctx, config.Load(), "fallback-svc")
	if err != nil {
		t.Fatal(err)
	}

	tr := otel.Tracer("pcmi.telemetry.test")
	_, span := tr.Start(ctx, "fake-work-unit")
	span.End()

	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := shutdown(sctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if atomic.LoadInt32(&posts) < 1 {
		t.Fatal("expected at least one OTLP/HTTP export to test server")
	}
}
