// Package metrics espone registry Prometheus dedicati (non DefaultRegisterer): Registry per l’API,
// WorkerRegistry per pcmi-worker. Contatori operazione memoria sull’API; contatore eventi Redis sul worker.
// /metrics API e worker usano promhttp sul rispettivo registry; evitare metriche duplicabili tra scrape
// in alta concorrenza (storia: CounterVec HTTP rimossi per gather stabile).
package metrics
