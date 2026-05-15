package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/embedding"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type MemoryService struct {
	repo     repository.MemoryRepository
	embedder embedding.Provider
}

func NewMemoryService(repo repository.MemoryRepository, embedder embedding.Provider) *MemoryService {
	return &MemoryService{repo: repo, embedder: embedder}
}

func (s *MemoryService) Store(ctx context.Context, req *model.StoreRequest, tenantID string) (*model.MemoryEntry, error) {
	log.Printf("📥 [SERVICE] Store — tenant=%s path=%s", tenantID, req.Path)

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

	log.Printf("📣 [REDIS] memory.stored id=%d", id)
	if err := event.PublishEvent(event.EventMemoryStored, map[string]any{
		"id":        entry.ID,
		"tenant_id": entry.TenantID,
		"path":      entry.Path,
	}); err != nil {
		log.Printf("❌ [REDIS] publish: %v", err)
	}

	return entry, nil
}

func (s *MemoryService) Retrieve(ctx context.Context, req *model.RetrieveRequest, tenantID string) (*model.RetrieveResponse, error) {
	var queryEmbedding []float32
	if q := strings.TrimSpace(req.Query); q != "" && s.embedder != nil {
		emb, err := s.embedder.Generate(ctx, q)
		if err != nil {
			log.Printf("⚠️ semantic retrieve fallback (embedding error): %v", err)
		} else {
			queryEmbedding = emb
		}
	}

	entries, err := s.repo.Retrieve(ctx, *req, tenantID, queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	if len(queryEmbedding) > 0 && len(entries) == 0 {
		entries, err = s.repo.Retrieve(ctx, *req, tenantID, nil)
		if err != nil {
			return nil, fmt.Errorf("retrieve fallback failed: %w", err)
		}
	}

	return &model.RetrieveResponse{
		Entries: entries,
		Total:   len(entries),
	}, nil
}
