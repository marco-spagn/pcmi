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

// Store salva un ricordo e pubblica l'evento memory.stored (v1.2 Event-Driven)
func (s *MemoryService) Store(ctx context.Context, m *model.Memory) (*model.Memory, error) {
	memory, err := s.repo.Store(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("store failed: %w", err)
	}

	// === v1.2 EVENT-DRIVEN: pubblica evento memory.stored ===
	event.GlobalBus.Publish(event.Event{
		Type: "memory.stored",
		Payload: map[string]any{
			"id":        memory.ID,
			"tenant_id": memory.TenantID,
			"path":      memory.Path,
		},
	})

	return memory, nil
}

// Retrieve delega al repository
func (s *MemoryService) Retrieve(ctx context.Context, query *model.RetrieveQuery) (*model.RetrieveResult, error) {
	result, err := s.repo.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	return result, nil
}
