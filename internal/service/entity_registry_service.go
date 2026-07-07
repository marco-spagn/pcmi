package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/model"
)

// EntityRegistryService resolves canonical entities and records evolution snapshots.
type EntityRegistryService struct {
	repo entityRegistryStore
}

type entityRegistryStore interface {
	ResolveCanonicalKey(ctx context.Context, tenantID, kind, aliasKey string) (string, error)
	UpsertCanonical(ctx context.Context, tenantID, kind, canonicalKey, displayName string, metadata map[string]interface{}) (*model.EntityRegistry, error)
	UpsertActiveAlias(ctx context.Context, tenantID, entityID, kind, aliasKey, source string, confidence float64, metadata map[string]interface{}) error
	InsertSnapshot(ctx context.Context, tenantID, entityID string, memoryID int64, memoryVersion int, profileID, slot, rawKey string, attributes map[string]interface{}, confidence float64) error
	ListEntities(ctx context.Context, tenantID, kind string, limit int) ([]model.EntityRegistry, error)
	GetByCanonical(ctx context.Context, tenantID, kind, canonicalKey string) (*model.EntityRegistry, error)
	GetByID(ctx context.Context, tenantID, id string) (*model.EntityRegistry, error)
	ListActiveAliases(ctx context.Context, tenantID, entityID string) ([]model.EntityAlias, error)
	ListSnapshots(ctx context.Context, tenantID, entityID string, limit int) ([]model.EntitySnapshot, error)
	ExpandEntityKeys(ctx context.Context, tenantID, kind, key string) ([]string, error)
	MergeEntityAliasInGraph(ctx context.Context, tenantID, kind, aliasKey, canonicalKey string) error
}

func NewEntityRegistryService(repo entityRegistryStore) *EntityRegistryService {
	return &EntityRegistryService{repo: repo}
}

// SyncFromExtraction registers canonical entities, aliases, snapshots, and graph mentions.
func (s *EntityRegistryService) SyncFromExtraction(
	ctx context.Context,
	tenantID string,
	entry *model.MemoryEntry,
	profile *extraction.Profile,
	rec *extraction.Record,
) ([]graph.EntityMention, error) {
	if s == nil || s.repo == nil || entry == nil || profile == nil || rec == nil || rec.Status != "ok" {
		return nil, nil
	}
	promoted := extraction.PromoteEntities(profile, rec)
	mentions := make([]graph.EntityMention, 0, len(promoted))
	for _, p := range promoted {
		rawKey := p.Key
		canonicalKey := rawKey
		useAliasTable := false
		if promo, ok := profile.EntityPromotion[p.Slot]; ok {
			useAliasTable = strings.EqualFold(strings.TrimSpace(promo.Normalize), "alias_table")
		}
		if useAliasTable {
			if resolved, err := s.repo.ResolveCanonicalKey(ctx, tenantID, p.Kind, rawKey); err == nil && resolved != "" {
				canonicalKey = resolved
			}
		}
		entity, err := s.repo.UpsertCanonical(ctx, tenantID, p.Kind, canonicalKey, rawKey, map[string]interface{}{
			"last_profile_id": profile.ProfileID,
			"last_memory_id":  entry.ID,
		})
		if err != nil {
			return mentions, err
		}
		if entity == nil {
			continue
		}
		if rawKey != canonicalKey {
			_ = s.repo.UpsertActiveAlias(ctx, tenantID, entity.ID, p.Kind, rawKey, model.EntityAliasSourceExtraction, p.Confidence, nil)
		}
		attrs := map[string]interface{}{
			"slot":         p.Slot,
			"raw_key":      rawKey,
			"canonical_key": canonicalKey,
		}
		if v, ok := rec.Slots[p.Slot]; ok {
			attrs["value"] = v
		}
		_ = s.repo.InsertSnapshot(ctx, tenantID, entity.ID, entry.ID, entry.Version, profile.ProfileID, p.Slot, rawKey, attrs, p.Confidence)
		mentions = append(mentions, graph.EntityMention{
			Slot:       p.Slot,
			Kind:       p.Kind,
			Key:        canonicalKey,
			Confidence: p.Confidence,
		})
	}
	return mentions, nil
}

func (s *EntityRegistryService) ListEntities(ctx context.Context, tenantID, kind string, limit int) ([]model.EntityRegistry, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("entity registry not configured")
	}
	return s.repo.ListEntities(ctx, tenantID, kind, limit)
}

func (s *EntityRegistryService) GetEntity(ctx context.Context, tenantID, kind, canonicalKey string) (*model.EntityRegistry, []model.EntityAlias, []model.EntitySnapshot, error) {
	if s == nil || s.repo == nil {
		return nil, nil, nil, fmt.Errorf("entity registry not configured")
	}
	entity, err := s.repo.GetByCanonical(ctx, tenantID, kind, canonicalKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if entity == nil {
		return nil, nil, nil, fmt.Errorf("entity not found")
	}
	aliases, err := s.repo.ListActiveAliases(ctx, tenantID, entity.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	snaps, err := s.repo.ListSnapshots(ctx, tenantID, entity.ID, 50)
	if err != nil {
		return nil, nil, nil, err
	}
	return entity, aliases, snaps, nil
}

func (s *EntityRegistryService) AddManualAlias(ctx context.Context, tenantID, kind, canonicalKey, aliasKey string, confidence float64) (*model.EntityRegistry, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("entity registry not configured")
	}
	aliasKey = extraction.NormalizeEntityKey(aliasKey, "trim")
	canonicalKey = extraction.NormalizeEntityKey(canonicalKey, "trim")
	if aliasKey == "" || canonicalKey == "" {
		return nil, fmt.Errorf("alias and canonical key are required")
	}
	entity, err := s.repo.UpsertCanonical(ctx, tenantID, kind, canonicalKey, canonicalKey, nil)
	if err != nil {
		return nil, err
	}
	if aliasKey != canonicalKey {
		if err := s.repo.UpsertActiveAlias(ctx, tenantID, entity.ID, kind, aliasKey, model.EntityAliasSourceManual, confidence, nil); err != nil {
			return nil, err
		}
		_ = s.repo.MergeEntityAliasInGraph(ctx, tenantID, kind, aliasKey, canonicalKey)
	}
	return entity, nil
}

func (s *EntityRegistryService) ExpandEntityKeys(ctx context.Context, tenantID, kind, key string) ([]string, error) {
	if s == nil || s.repo == nil {
		return []string{strings.TrimSpace(key)}, nil
	}
	return s.repo.ExpandEntityKeys(ctx, tenantID, kind, key)
}

func (s *EntityRegistryService) ApplyAliasMerge(ctx context.Context, tenantID, kind, aliasKey, canonicalKey string, source string, confidence float64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("entity registry not configured")
	}
	entity, err := s.repo.GetByCanonical(ctx, tenantID, kind, canonicalKey)
	if err != nil {
		return err
	}
	if entity == nil {
		entity, err = s.repo.UpsertCanonical(ctx, tenantID, kind, canonicalKey, canonicalKey, nil)
		if err != nil {
			return err
		}
	}
	if err := s.repo.UpsertActiveAlias(ctx, tenantID, entity.ID, kind, aliasKey, source, confidence, nil); err != nil {
		return err
	}
	return s.repo.MergeEntityAliasInGraph(ctx, tenantID, kind, aliasKey, canonicalKey)
}
