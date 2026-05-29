// Package metrics exposes dedicated Prometheus registries (not DefaultRegisterer): Registry for the API,
// WorkerRegistry for pcmi-worker. Memory operation counters on the API; Redis event counter on the worker.
// API and worker /metrics use promhttp against their respective registry; avoid duplicatable metrics across
// high-concurrency scrapes (history: HTTP CounterVec removed for stable gather).
package metrics
