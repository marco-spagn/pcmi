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
	repo     repository.MemoryRepo
	embedder embedding.Provider
}

type StoreResult struct {
	Entry        *model.MemoryEntry
	Version      int
	SupersededID *int64
}

func NewMemoryService(repo repository.MemoryRepo, embedder embedding.Provider) *MemoryService {
	return &MemoryService{repo: repo, embedder: embedder}
}

func (s *MemoryService) Store(ctx context.Context, req *model.StoreRequest, tenantID string) (*StoreResult, error) {
	log.Printf("📥 [SERVICE] Store — tenant=%s path=%s", tenantID, req.Path)

	id, version, supersededID, err := s.repo.Store(ctx, *req, tenantID)
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
		Version:        version,
		ValidFrom:      time.Now(),
		CreatedAt:      time.Now(),
		Importance:     model.NormalizeImportance(req.Importance),
	}

	payload := map[string]any{
		"id":        entry.ID,
		"tenant_id": entry.TenantID,
		"path":      entry.Path,
		"version":   version,
	}
	eventType := event.EventMemoryStored
	if supersededID != nil {
		eventType = event.EventMemoryUpdated
		payload["superseded_id"] = *supersededID
		log.Printf("📣 [REDIS] memory.updated id=%d version=%d superseded=%d", id, version, *supersededID)
	} else {
		log.Printf("📣 [REDIS] memory.stored id=%d version=%d", id, version)
	}

	if err := event.PublishEvent(eventType, payload); err != nil {
		log.Printf("❌ [REDIS] publish: %v", err)
	}

	return &StoreResult{
		Entry:        entry,
		Version:      version,
		SupersededID: supersededID,
	}, nil
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

	pathOnly := strings.TrimSpace(req.Query) == ""
	entries, err := s.repo.Retrieve(ctx, *req, tenantID, queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	if len(queryEmbedding) > 0 && len(entries) == 0 {
		pathOnly = false
		entries, err = s.repo.Retrieve(ctx, *req, tenantID, nil)
		if err != nil {
			return nil, fmt.Errorf("retrieve fallback failed: %w", err)
		}
	} else if len(queryEmbedding) > 0 {
		pathOnly = false
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}

	resp := &model.RetrieveResponse{
		Entries: entries,
		Total:   len(entries),
	}
	if pathOnly && len(entries) > limit {
		resp.HasMore = true
		resp.Entries = entries[:limit]
		resp.Total = len(resp.Entries)
		last := resp.Entries[len(resp.Entries)-1]
		page, pageErr := model.MakeNextCursor(model.SortKeyCreatedAtIDDesc, last.ID, last.CreatedAt, true)
		if pageErr != nil {
			return nil, pageErr
		}
		resp.NextCursor = page.NextCursor
	}
	return resp, nil
}

func (s *MemoryService) Rollback(ctx context.Context, req *model.RollbackRequest, tenantID string) (*model.RollbackResponse, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	historical, err := s.repo.GetHistoricalVersion(ctx, tenantID, path, req.Version, req.AsOf)
	if err != nil {
		return nil, err
	}

	meta, _ := historical.Metadata.(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
	}

	storeReq := model.StoreRequest{
		Path:           path,
		Content:        historical.Content,
		Metadata:       meta,
		Tags:           historical.Tags,
		EmbeddingModel: historical.EmbeddingModel,
		EmbeddingSpace: historical.EmbeddingSpace,
		Embedding:      historical.Embedding,
	}
	if historical.SourceAgentID != nil {
		storeReq.SourceAgentID = *historical.SourceAgentID
	}

	result, err := s.Store(ctx, &storeReq, tenantID)
	if err != nil {
		return nil, err
	}

	return &model.RollbackResponse{
		ID:                  result.Entry.ID,
		Status:              "rolled_back",
		Version:             result.Version,
		RestoredFromVersion: historical.Version,
		SupersededID:        result.SupersededID,
	}, nil
}

// Compact trims superseded history for one path (tenant-scoped).
func (s *MemoryService) Compact(ctx context.Context, tenantID string, req *model.CompactMemoryRequest) (*model.CompactMemoryResponse, error) {
	keep := req.KeepSuperseded
	if keep <= 0 {
		keep = 20
	}
	n, err := s.repo.CompactPathHistory(ctx, tenantID, req.Path, keep)
	if err != nil {
		return nil, err
	}
	return &model.CompactMemoryResponse{
		Path:           strings.TrimSpace(req.Path),
		DeletedCount:   n,
		KeepSuperseded: keep,
	}, nil
}

func (s *MemoryService) GetByPath(ctx context.Context, tenantID, path string, version *int, asOf *time.Time) (*model.MemoryEntry, error) {
	return s.repo.GetByPath(ctx, tenantID, path, version, asOf)
}

// UpdateImportance sets importance on the current row at path.
func (s *MemoryService) UpdateImportance(ctx context.Context, tenantID, path string, importance float64) error {
	if err := model.ValidateImportance(importance); err != nil {
		return err
	}
	return s.repo.UpdateImportance(ctx, tenantID, path, importance)
}

func (s *MemoryService) Export(ctx context.Context, tenantID string, req *model.MemoryExportRequest) (*model.MemoryExportResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	entries, err := s.repo.ExportMemories(ctx, tenantID, req.PathPrefix, limit, req.IncludeEmb)
	if err != nil {
		return nil, err
	}
	return &model.MemoryExportResponse{
		TenantID:   tenantID,
		Exported:   len(entries),
		Entries:    entries,
		ExportedAt: time.Now(),
	}, nil
}

func (s *MemoryService) Import(ctx context.Context, tenantID string, req *model.MemoryImportRequest) (*model.MemoryImportResponse, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "skip"
	}
	out := &model.MemoryImportResponse{}
	for i, item := range req.Entries {
		if mode == "skip" {
			existing, err := s.repo.GetByPath(ctx, tenantID, item.Path, nil, nil)
			if err == nil && existing != nil {
				out.Skipped++
				out.Results = append(out.Results, model.BatchStoreItemResult{Index: i, Status: "skipped"})
				continue
			}
		}
		res, err := s.Store(ctx, &item, tenantID)
		if err != nil {
			out.Results = append(out.Results, model.BatchStoreItemResult{Index: i, Status: "error", Error: err.Error()})
			continue
		}
		out.Imported++
		out.Results = append(out.Results, model.BatchStoreItemResult{
			Index: i, ID: res.Entry.ID, Status: "stored", Version: res.Version,
		})
	}
	return out, nil
}
