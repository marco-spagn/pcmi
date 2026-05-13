package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type MemoryService struct {
	repo repository.MemoryRepository
}

func NewMemoryService(repo repository.MemoryRepository) *MemoryService {
	return &MemoryService{repo: repo}
}

func (s *MemoryService) Store(ctx context.Context, req *model.StoreRequest) (*model.MemoryEntry, error) {
	log.Printf("📥 [SERVICE] Store chiamato - path: %s", req.Path)

	id, err := s.repo.Store(ctx, *req)
	if err != nil {
		log.Printf("❌ [SERVICE] repo.Store fallito: %v", err)
		return nil, fmt.Errorf("store failed: %w", err)
	}
	log.Printf("✅ [SERVICE] repo.Store OK - id: %d", id)

	entry := &model.MemoryEntry{
		ID:             id,
		TenantID:       req.TenantID,
		Path:           req.Path,
		Content:        req.Content,
		Metadata:       req.Metadata,
		Tags:           req.Tags,
		EmbeddingModel: req.EmbeddingModel,
		Version:        1,
		ValidFrom:      time.Now(),
		CreatedAt:      time.Now(),
	}

	// === v1.3 REDIS EVENT ===
	log.Printf("📣 [REDIS] Tentativo pubblicazione per id=%d", id)
	err = event.PublishEvent("memory.stored", map[string]any{
		"id":        entry.ID,
		"tenant_id": entry.TenantID,
		"path":      entry.Path,
	})
	if err != nil {
		log.Printf("❌ [REDIS] ERRORE: %v", err)
	} else {
		log.Printf("✅ [REDIS] Pubblicato con successo")
	}

	return entry, nil
}

func (s *MemoryService) Retrieve(ctx context.Context, req *model.RetrieveRequest) (*model.RetrieveResponse, error) {
	entries, err := s.repo.Retrieve(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}

	return &model.RetrieveResponse{
		Entries: entries,
		Total:   len(entries),
	}, nil
}
