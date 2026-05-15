package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/model"
)

type LineageRepository struct {
	db *pgxpool.Pool
}

func NewLineageRepository(db *pgxpool.Pool) *LineageRepository {
	return &LineageRepository{db: db}
}

func (r *LineageRepository) MemoryLineage(ctx context.Context, tenantID, path string) (*model.MemoryLineageResponse, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	memRepo := NewMemoryRepository(r.db)
	versions, err := memRepo.ListPathHistory(ctx, tenantID, path, 100)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("memory not found")
	}

	var distilled []model.DistilledLineageItem
	rows, err := r.db.Query(ctx, `
		SELECT id, path::text, summary, version, source_entry_ids, confidence_score
		FROM distilled_knowledge
		WHERE tenant_id = $1::uuid AND path <@ $2::ltree
		ORDER BY version DESC, distilled_at DESC
		LIMIT 20`, tenantID, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d model.DistilledLineageItem
		if err := rows.Scan(&d.ID, &d.Path, &d.Summary, &d.Version, &d.SourceEntryIDs, &d.ConfidenceScore); err != nil {
			return nil, err
		}
		distilled = append(distilled, d)
	}
	if distilled == nil {
		distilled = []model.DistilledLineageItem{}
	}

	return &model.MemoryLineageResponse{
		EntryID:   versions[0].ID,
		Path:      path,
		Versions:  versions,
		Distilled: distilled,
	}, nil
}

func (r *LineageRepository) DistilledLineage(ctx context.Context, tenantID string, distilledID int64) (*model.DistilledLineageResponse, error) {
	var item model.DistilledLineageItem
	err := r.db.QueryRow(ctx, `
		SELECT id, path::text, summary, version, source_entry_ids, confidence_score
		FROM distilled_knowledge
		WHERE tenant_id = $1::uuid AND id = $2`,
		tenantID, distilledID,
	).Scan(&item.ID, &item.Path, &item.Summary, &item.Version, &item.SourceEntryIDs, &item.ConfidenceScore)
	if err != nil {
		return nil, fmt.Errorf("distilled not found")
	}

	var sources []model.MemoryEntry
	if len(item.SourceEntryIDs) > 0 {
		rows, qErr := r.db.Query(ctx, `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted,
			       NULL::float8
			FROM memory_entries
			WHERE tenant_id = $1::uuid AND id = ANY($2::bigint[])`,
			tenantID, item.SourceEntryIDs)
		if qErr != nil {
			return nil, qErr
		}
		defer rows.Close()
		memRepo := &MemoryRepository{db: r.db}
		for rows.Next() {
			e, scanErr := memRepo.scanMemoryEntry(rows, false)
			if scanErr != nil {
				return nil, scanErr
			}
			sources = append(sources, e)
		}
	}
	if sources == nil {
		sources = []model.MemoryEntry{}
	}

	return &model.DistilledLineageResponse{Distilled: item, Sources: sources}, nil
}
