# Runbook: Distillation Lag

## Symptoms
- Alert `PCMISLODistillationLag` fires: `pcmi_distillation_queued_jobs > 50` for 5+ minutes.
- Memory store events not producing distillation results within 60 seconds.
- `pcmi_distillation_job_duration_seconds` histogram shows long tail or no recent observations.

## Probable causes
1. **OpenAI rate limiting (429)** — distillation LLM calls are being throttled; jobs queue up waiting for retries.
2. **DISTILLATION_CONCURRENCY too low** — the semaphore is saturated; new jobs block until a slot frees.
3. **Worker process crashed or restarted** — in-flight goroutines lost; queue shows backlog from missed events.
4. **Redis pub/sub broken** — `memory.stored` events not reaching the worker; queue grows from manual triggers only.
5. **OpenAI API outage** — all LLM calls failing; jobs exhaust retries and land in error state.

## Investigation steps

Check current queue depth:
```promql
pcmi_distillation_queued_jobs
```

Check distillation error rate:
```promql
sum(rate(pcmi_distillation_total{status="error"}[5m]))
```

Check active vs queued distillation jobs:
```promql
pcmi_distillation_active_jobs
pcmi_distillation_queued_jobs
```

Check worker Redis event consumption rate:
```promql
sum(rate(pcmi_worker_redis_events_total[5m]))
```

Check P95 distillation job duration:
```promql
histogram_quantile(0.95,
  sum by (le) (rate(pcmi_distillation_job_duration_seconds_bucket[5m]))
)
```

Check worker logs for OpenAI errors:
```bash
kubectl logs -l app=pcmi-worker --tail=200 | grep -i "429\|rate limit\|openai"
```

## Remediation steps
1. **OpenAI 429**: reduce `DISTILLATION_CONCURRENCY` to lower request rate; add exponential backoff config if not already set.
2. **Low concurrency**: increase `DISTILLATION_CONCURRENCY` in worker deployment env vars (max 16); redeploy worker.
3. **Worker crash**: `kubectl rollout restart deployment/pcmi-worker`; verify Redis subscription resumes.
4. **Redis broken**: check Redis connectivity from worker pod; restart Redis if cluster health is degraded.
5. **OpenAI outage**: check status.openai.com; enable distillation circuit-breaker in config to prevent queue pileup.

## Escalation path
1. On-call engineer: check OpenAI status page and worker logs for root cause within 10 minutes of alert.
2. Worker team: investigate Redis pub/sub if worker is running but event consumption is zero.
3. Platform team: if queue > 200 jobs and growing, consider temporarily disabling distillation triggers to prevent worker OOM, then drain queue after the upstream issue resolves.
