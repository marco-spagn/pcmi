package handler

import (
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/worker"
)

// ConfigureReranker enables LLM reranking on svc when RERANK_ENABLED is set
// and the LLM_PROVIDER client has credentials. It is a no-op otherwise, so a
// misconfigured reranker never breaks retrieval.
func ConfigureReranker(svc *service.MemoryService, cfg *config.Config) {
	if svc == nil || cfg == nil || !cfg.RerankEnabled {
		return
	}
	llmCfg := *cfg
	if cfg.RerankModel != "" {
		llmCfg.DistillationModel = cfg.RerankModel
	}
	llm, err := worker.NewLLMClient(&llmCfg)
	if err != nil || llm == nil || !llm.IsConfigured() {
		log.Printf("⚠️  RERANK_ENABLED=true but the LLM_PROVIDER client is not configured (%v) — reranking disabled", err)
		return
	}
	svc.SetReranker(
		service.NewLLMReranker(llm, time.Duration(cfg.RerankTimeoutMs)*time.Millisecond),
		service.RerankConfig{Candidates: cfg.RerankCandidates},
	)
}
