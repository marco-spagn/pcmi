package repository

import (
	"context"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

// MemoryRepo is the persistence surface used by MemoryService.
type MemoryRepo interface {
	Store(ctx context.Context, req model.StoreRequest, tenantID string) (id int64, version int, supersededID *int64, err error)
	Retrieve(ctx context.Context, req model.RetrieveRequest, tenantID string, queryEmbedding []float32) ([]model.MemoryEntry, error)
	GetHistoricalVersion(ctx context.Context, tenantID, path string, version *int, asOf *time.Time) (*model.MemoryEntry, error)
	GetByPath(ctx context.Context, tenantID, path string, version *int, asOf *time.Time) (*model.MemoryEntry, error)
	ExportMemories(ctx context.Context, tenantID, pathPrefix string, limit int, includeEmb bool) ([]model.MemoryEntry, error)
	CompactPathHistory(ctx context.Context, tenantID, path string, keepSuperseded int) (deleted int, err error)
}
