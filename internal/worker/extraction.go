package worker

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

// ExtractionWorker runs LLM attribute extraction after memory store/update events.
type ExtractionWorker struct {
	svc       *service.ExtractionService
	proposals *service.LinkProposalService
}

// NewExtractionWorker wires the extraction service for async worker use.
func NewExtractionWorker(db *pgxpool.Pool, cfg *config.Config) *ExtractionWorker {
	graphClient := graph.NewGraphClient(db)
	profiles := repository.NewExtractionRepository(db, nil)
	memRepo := repository.NewMemoryRepository(db, nil)
	links := repository.NewLinksRepository(db, nil)
	proposalsRepo := repository.NewLinkProposalRepository(db, nil)
	llm, _ := NewLLMClient(cfg)
	svc := service.NewExtractionService(profiles, memRepo, llm, cfg, graphClient)
	proposals := service.NewLinkProposalService(proposalsRepo, profiles, links, graphClient, llm, cfg)
	return &ExtractionWorker{svc: svc, proposals: proposals}
}

// Enabled reports whether extraction is turned on in config.
func (w *ExtractionWorker) Enabled() bool {
	return w != nil && w.svc != nil && w.svc.Enabled()
}

// OnMemoryEvent schedules extraction for a stored/updated memory.
func (w *ExtractionWorker) OnMemoryEvent(tenantID, path string, memoryID int64, version int) {
	if w == nil || !w.Enabled() || tenantID == "" || path == "" || memoryID <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := w.svc.ExtractPath(ctx, tenantID, path, memoryID, version); err != nil {
			log.Printf("extraction worker: tenant=%s path=%s id=%d: %v", tenantID, path, memoryID, err)
			return
		}
		if w.proposals != nil && w.proposals.Enabled() {
			if _, err := w.proposals.GenerateForMemory(ctx, tenantID, memoryID); err != nil {
				log.Printf("link proposal worker: tenant=%s id=%d: %v", tenantID, memoryID, err)
			}
		}
	}()
}
