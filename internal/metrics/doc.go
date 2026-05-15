// Package metrics espone un registry Prometheus dedicato (non DefaultRegisterer) con contatori
// operazione memoria. /metrics è servito con promhttp sullo stesso registry; evitare metriche
// duplicabili tra scrape in alta concorrenza (storia: CounterVec HTTP rimossi per gather stabile).
package metrics
