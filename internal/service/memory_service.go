package service

import (
	"context"
	"fmt"

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

// Store usa StoreRequest e pubblica l'evento (v1.2)
func (s *MemoryService) Store(ctx context.Context, req *model.StoreRequest) (*model.MemoryEntry, error) {
	id, err := s.repo.Store(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("store failed: %w", err)
	}

	// Creiamo l'entry da restituire
	entry := &model.MemoryEntry{
		ID:       id,
		TenantID: req.TenantID,
		Path:     req.Path,
	}

	// === v1.2 EVENT-DRIVEN ===
	event.GlobalBus.Publish(event.Event{
		Type: "memory.stored",
		Payload: map[string]any{
			"id":        entry.ID,
			"tenant_id": entry.TenantID,
			"path":      entry.Path,
		},
	})

	return entry, nil
}

// Retrieve usa RetrieveRequest
func (s *MemoryService) Retrieve(ctx context.Context, req *model.RetrieveRequest) ([]model.MemoryEntry, error) {
	result, err := s.repo.Retrieve(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	return result, nil
}
