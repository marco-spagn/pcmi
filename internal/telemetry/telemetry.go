// Package telemetry configures OpenTelemetry global propagators and an optional
// OTLP/HTTP trace exporter. Prometheus metrics remain separate (see internal/metrics).
package telemetry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace/noop"
)

// Init configures W3C propagators (tracecontext + baggage) and an optional OTLP/HTTP trace exporter.
// OTLP settings are read from cfg (loaded via config.Load at the process entrypoint).
// If neither traces nor generic OTLP endpoint is set, a noop tracer provider is installed.
// cfg.OTELServiceName overrides defaultServiceName when set.
// The returned shutdown function should be called on exit (with a timeout context).
func Init(ctx context.Context, cfg *config.Config, defaultServiceName string) (shutdown func(context.Context) error, err error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := ""
	serviceName := ""
	if cfg != nil {
		endpoint = strings.TrimSpace(cfg.OTELTracesEndpoint)
		if endpoint == "" {
			endpoint = strings.TrimSpace(cfg.OTELEndpoint)
		}
		serviceName = strings.TrimSpace(cfg.OTELServiceName)
	}
	if serviceName == "" {
		serviceName = strings.TrimSpace(defaultServiceName)
	}
	if serviceName == "" {
		serviceName = "pcmi-api"
	}

	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	log.Info("otel configured", "endpoint", endpoint, "service", serviceName)
	return tp.Shutdown, nil
}
