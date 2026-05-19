package grpcserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/metrics"
)

const metricsScrapeChunkSize = 64 * 1024

type metricsServer struct {
	pcmiv1.UnimplementedMetricsServiceServer
	auth *memoryServer
}

func newMetricsServer(db *pgxpool.Pool) *metricsServer {
	return &metricsServer{auth: &memoryServer{db: db}}
}

func gatherPrometheusBody(openMetrics bool) ([]byte, string, error) {
	rec := httptest.NewRecorder()
	promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: openMetrics,
	}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		return nil, "", fmt.Errorf("scrape HTTP %d", rec.Code)
	}
	return rec.Body.Bytes(), rec.Header().Get("Content-Type"), nil
}

func (s *metricsServer) Scrape(ctx context.Context, req *pcmiv1.ScrapeRequest) (*pcmiv1.ScrapeResponse, error) {
	if _, _, err := s.auth.resolveTenantAndRole(ctx, ""); err != nil {
		return nil, err
	}
	body, ct, err := gatherPrometheusBody(req.GetOpenMetrics())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "scrape: %v", err)
	}
	return &pcmiv1.ScrapeResponse{Body: body, ContentType: ct}, nil
}

func (s *metricsServer) StreamScrape(req *pcmiv1.ScrapeRequest, stream pcmiv1.MetricsService_StreamScrapeServer) error {
	ctx := stream.Context()
	if _, _, err := s.auth.resolveTenantAndRole(ctx, ""); err != nil {
		return err
	}
	body, _, err := gatherPrometheusBody(req.GetOpenMetrics())
	if err != nil {
		return status.Errorf(codes.Internal, "scrape: %v", err)
	}
	for off := 0; off < len(body) || off == 0; {
		end := off + metricsScrapeChunkSize
		if end > len(body) {
			end = len(body)
		}
		chunk := body[off:end]
		last := end >= len(body)
		if err := stream.Send(&pcmiv1.ScrapeChunk{Body: chunk, Last: last}); err != nil {
			return err
		}
		if last {
			break
		}
		off = end
	}
	return nil
}

func (s *metricsServer) GetMetric(context.Context, *pcmiv1.GetMetricRequest) (*pcmiv1.GetMetricResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetMetric not implemented; use Scrape")
}
