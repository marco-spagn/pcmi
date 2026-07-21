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
	GetByIDResolveCurrent(ctx context.Context, tenantID string, memoryID int64) (*model.MemoryEntry, int64, error)
	ExportMemories(ctx context.Context, tenantID, pathPrefix string, limit int, includeEmb bool) ([]model.MemoryEntry, error)
	CompactPathHistory(ctx context.Context, tenantID, path string, keepSuperseded int) (deleted int, err error)
	UpdateImportance(ctx context.Context, tenantID, path string, importance float64) error
	GetTenantDedupMode(ctx context.Context, tenantID string) (model.DedupMode, error)
	FindCurrentByContentHash(ctx context.Context, tenantID, hash string) (*model.MemoryEntry, error)
	MergeCurrentMetadata(ctx context.Context, tenantID, path string, metadata map[string]interface{}, tags []string) (*model.MemoryEntry, error)
	UpsertDedupLink(ctx context.Context, tenantID, fromPath, toPath string) error
}
