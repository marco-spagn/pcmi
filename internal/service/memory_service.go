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

func (s *MemoryService) Store(ctx context.Context, req *model.StoreRequest, tenantID string) (*model.MemoryEntry, error) {
	log.Printf("📥 [SERVICE] Store chiamato - tenant=%s, path=%s", tenantID, req.Path)

	id, err := s.repo.Store(ctx, *req, tenantID)
	if err != nil {
		return nil, fmt.Errorf("store failed: %w", err)
	}

	entry := &model.MemoryEntry{
		ID:             id,
		TenantID:       tenantID,
		Path:           req.Path,
		Content:        req.Content,
		Metadata:       req.Metadata,
		Tags:           req.Tags,
		EmbeddingModel: req.EmbeddingModel,
		Version:        1,
		ValidFrom:      time.Now(),
		CreatedAt:      time.Now(),
	}

	// v1.3 Redis Event
	log.Printf("📣 [REDIS] Pubblicazione evento memory.stored per id=%d", id)
	err = event.PublishEvent("memory.stored", map[string]any{
		"id":        entry.ID,
		"tenant_id": entry.TenantID,
		"path":      entry.Path,
	})
	if err != nil {
		log.Printf("❌ [REDIS] ERRORE pubblicazione: %v", err)
	} else {
		log.Printf("✅ [REDIS] Pubblicato con successo")
	}

	return entry, nil
}

func (s *MemoryService) Retrieve(ctx context.Context, req *model.RetrieveRequest, tenantID string) (*model.RetrieveResponse, error) {
	entries, err := s.repo.Retrieve(ctx, *req, tenantID)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}

	return &model.RetrieveResponse{
		Entries: entries,
		Total:   len(entries),
	}, nil
}
