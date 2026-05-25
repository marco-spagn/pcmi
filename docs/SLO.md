# PCMI Service Level Objectives

## SLO definitions (v1.49+)

### API availability
- SLO: 99.9% uptime measured over 30-day rolling window
- SLI: (successful requests / total requests) where successful = HTTP status < 500
- PromQL: 
  sum(rate(pcmi_http_requests_total{status!~"5.."}[5m])) /
  sum(rate(pcmi_http_requests_total[5m]))
- Error budget: 43.8 min/month

### Store latency (P99)
- SLO: P99 < 50ms at ≤ 500 req/s sustained load (single API pod, PgBouncer pool=20)
- SLI: histogram_quantile(0.99, rate(pcmi_http_request_duration_seconds_bucket{handler="store"}[5m]))
- Alert: fires when P99 > 45ms for 5 min (5ms headroom before SLO breach)

### Retrieve latency (P99) — hybrid mode
- SLO: P99 < 100ms for path_prefix scoped queries, limit ≤ 20, corpus ≤ 500k entries/tenant
- SLI: histogram_quantile(0.99, rate(pcmi_http_request_duration_seconds_bucket{handler="retrieve"}[5m]))
- Alert: fires when P99 > 90ms for 5 min

### Distillation lag
- SLO: 95% of memory.stored events produce a distillation result within 60s
- SLI: measured by pcmi_distillation_job_duration_seconds histogram
- Alert: pcmi_distillation_queued_jobs > 50 for > 5 min

### Webhook delivery
- SLO: 99% of webhook deliveries succeed within 5 attempts over 30 min
- SLI: 1 - (dead_letter_total / total_attempts_total) over 1h
- Alert: already defined in deploy/prometheus/alerts.yaml

## Runbook links
- Store P99 high: docs/runbooks/store-latency.md
- Retrieve P99 high: docs/runbooks/retrieve-latency.md
- Distillation lag: docs/runbooks/distillation-lag.md
