// Package usage meters LLM and embedding token consumption per tenant and
// operation (FinOps).
//
// Call sites tag their context with WithScope(ctx, tenantID, operation); the
// provider clients (internal/worker LLM clients, internal/embedding, the
// summarize and rerank services) call Report with the token counts returned by
// the upstream API. Every report increments the Prometheus counters
// pcmi_llm_requests_total / pcmi_llm_tokens_total (labels: provider, model,
// operation — no tenant label, to bound cardinality) and, when a Recorder is
// installed with SetRecorder, is aggregated per tenant and UTC day and flushed
// to llm_usage_daily (migration 029) by Aggregator.
package usage

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/marco-spagn/pcmi/internal/metrics"
)

// Operation names used by PCMI call sites.
const (
	OpEmbedding       = "embedding"       // background embedding of stored memories
	OpQueryEmbedding  = "query_embedding" // semantic retrieve query vector
	OpDistillation    = "distillation"    // worker distillation
	OpExtraction      = "extraction"      // entity slot extraction
	OpLinkProposal    = "link_proposal"   // LLM link proposals
	OpEntityAlias     = "entity_alias"    // entity alias proposals
	OpSummarize       = "summarize"       // POST /v1/memories/summarize
	OpRerank          = "rerank"          // LLM reranking of retrieve results
	OpContradiction   = "contradiction"   // contradiction detection
	OpUnknown         = "unknown"         // untagged context
	maxLabelValueSize = 64
)

// Event is one metered upstream call.
type Event struct {
	TenantID     string
	Operation    string
	Provider     string
	Model        string
	InputTokens  int64
	OutputTokens int64
	At           time.Time
}

// Recorder receives every reported Event (e.g. *Aggregator).
type Recorder interface {
	Record(Event)
}

type scopeKey struct{}

type scope struct {
	tenantID  string
	operation string
}

// WithScope tags ctx with the tenant and operation that upstream calls made
// under it are billed to.
func WithScope(ctx context.Context, tenantID, operation string) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope{tenantID: tenantID, operation: operation})
}

// ScopeFrom returns the tenant and operation tagged on ctx ("" when absent).
func ScopeFrom(ctx context.Context) (tenantID, operation string) {
	if ctx == nil {
		return "", ""
	}
	s, _ := ctx.Value(scopeKey{}).(scope)
	return s.tenantID, s.operation
}

var (
	requestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pcmi_llm_requests_total",
		Help: "Metered LLM / embedding upstream calls by provider, model and operation.",
	}, []string{"provider", "model", "operation"})
	tokensTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pcmi_llm_tokens_total",
		Help: "LLM / embedding tokens by provider, model, operation and direction (input|output).",
	}, []string{"provider", "model", "operation", "direction"})

	recMu    sync.RWMutex
	recorder Recorder
)

func init() {
	metrics.Registry.MustRegister(requestsTotal, tokensTotal)
	metrics.WorkerRegistry.MustRegister(requestsTotal, tokensTotal)
}

// SetRecorder installs the process-wide recorder (nil disables persistence;
// Prometheus counters are always updated). It returns the previous recorder.
func SetRecorder(r Recorder) Recorder {
	recMu.Lock()
	defer recMu.Unlock()
	prev := recorder
	recorder = r
	return prev
}

func label(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	if len(v) > maxLabelValueSize {
		v = v[:maxLabelValueSize]
	}
	return v
}

// Report meters one upstream call made under ctx. Negative token counts are
// clamped to zero. Safe for concurrent use.
func Report(ctx context.Context, provider, model string, inputTokens, outputTokens int64) {
	tenantID, op := ScopeFrom(ctx)
	ev := Event{
		TenantID:     tenantID,
		Operation:    label(op, OpUnknown),
		Provider:     label(provider, "unknown"),
		Model:        label(model, "unknown"),
		InputTokens:  max(inputTokens, 0),
		OutputTokens: max(outputTokens, 0),
		At:           time.Now().UTC(),
	}
	requestsTotal.WithLabelValues(ev.Provider, ev.Model, ev.Operation).Inc()
	tokensTotal.WithLabelValues(ev.Provider, ev.Model, ev.Operation, "input").Add(float64(ev.InputTokens))
	tokensTotal.WithLabelValues(ev.Provider, ev.Model, ev.Operation, "output").Add(float64(ev.OutputTokens))

	recMu.RLock()
	r := recorder
	recMu.RUnlock()
	if r != nil {
		r.Record(ev)
	}
}
