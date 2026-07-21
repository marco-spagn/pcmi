package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/entityalias"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

// EntityAliasProposalService generates and reviews entity alias merge proposals.
type EntityAliasProposalService struct {
	proposals entityAliasProposalStore
	registry  entityAliasRegistryStore
	profiles  entityAliasProfileStore
	llm       LLMCompleter
	entities  *EntityRegistryService
	model     string
	enabled   bool
}

type entityAliasProposalStore interface {
	InsertPending(ctx context.Context, tenantID string, p repository.InsertEntityAliasProposalParams) (*model.EntityAliasProposal, error)
	GetByID(ctx context.Context, tenantID string, id int64) (*model.EntityAliasProposal, error)
	List(ctx context.Context, tenantID, status string, limit int) ([]model.EntityAliasProposal, error)
	UpdateStatus(ctx context.Context, tenantID string, id int64, status string) (*model.EntityAliasProposal, error)
}

type entityAliasRegistryStore interface {
	GetByCanonical(ctx context.Context, tenantID, kind, canonicalKey string) (*model.EntityRegistry, error)
	GetByID(ctx context.Context, tenantID, id string) (*model.EntityRegistry, error)
	ResolveCanonicalKey(ctx context.Context, tenantID, kind, aliasKey string) (string, error)
	ListEntities(ctx context.Context, tenantID, kind string, limit int) ([]model.EntityRegistry, error)
}

type entityAliasProfileStore interface {
	GetCurrentMemoryByID(ctx context.Context, tenantID string, memoryID int64) (*model.MemoryEntry, error)
	MatchProfile(ctx context.Context, tenantID, path string) (*extraction.Profile, string, error)
}

func NewEntityAliasProposalService(
	proposals entityAliasProposalStore,
	registry entityAliasRegistryStore,
	entities *EntityRegistryService,
	profiles entityAliasProfileStore,
	llm LLMCompleter,
	cfg *config.Config,
) *EntityAliasProposalService {
	s := &EntityAliasProposalService{
		proposals: proposals,
		registry:  registry,
		entities:  entities,
		profiles:  profiles,
		llm:       llm,
		model:     "gpt-4o-mini",
	}
	if cfg != nil {
		s.enabled = cfg.EntityAliasProposalsEnabled
		if m := strings.TrimSpace(cfg.ExtractionModel); m != "" {
			s.model = m
		}
	}
	return s
}

func (s *EntityAliasProposalService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *EntityAliasProposalService) List(ctx context.Context, tenantID, status string, limit int) ([]model.EntityAliasProposal, error) {
	return s.proposals.List(ctx, tenantID, status, limit)
}

func (s *EntityAliasProposalService) GenerateForMemory(ctx context.Context, tenantID string, memoryID int64) ([]model.EntityAliasProposal, error) {
	if s == nil || !s.enabled || s.llm == nil || !s.llm.IsConfigured() {
		return nil, fmt.Errorf("entity alias proposals are disabled or LLM not configured")
	}
	entry, err := s.profiles.GetCurrentMemoryByID(ctx, tenantID, memoryID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("memory not found")
	}
	rec, ok := extraction.RecordFromMetadata(metadataAsMap(entry.Metadata))
	if !ok || rec == nil || rec.Status != "ok" {
		return nil, fmt.Errorf("memory has no successful extraction")
	}
	profile, _, err := s.profiles.MatchProfile(ctx, tenantID, entry.Path)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("no matching extraction profile")
	}

	promoted := extraction.PromoteEntities(profile, rec)
	var stored []model.EntityAliasProposal
	for _, p := range promoted {
		sourceEntity, err := s.registry.GetByCanonical(ctx, tenantID, p.Kind, p.Key)
		if err != nil || sourceEntity == nil {
			canonical := p.Key
			if resolved, _ := s.registry.ResolveCanonicalKey(ctx, tenantID, p.Kind, p.Key); resolved != "" {
				canonical = resolved
			}
			sourceEntity, err = s.registry.GetByCanonical(ctx, tenantID, p.Kind, canonical)
			if err != nil || sourceEntity == nil {
				continue
			}
		}
		candidatesRows, err := s.registry.ListEntities(ctx, tenantID, p.Kind, 30)
		if err != nil {
			return stored, err
		}
		candidates := entityalias.FromRegistryRows(candidatesRows, sourceEntity.ID)
		if len(candidates) == 0 {
			continue
		}
		allowed := make(map[string]struct{}, len(candidates))
		for _, c := range candidates {
			allowed[c.ID] = struct{}{}
		}
		raw, err := s.llm.Complete(ctx, entityalias.BuildSystemPrompt(profile),
			[]string{entityalias.BuildUserMessage(p.Kind, p.Key, sourceEntity.ID, candidates)})
		if err != nil {
			return stored, fmt.Errorf("llm entity alias proposals: %w", err)
		}
		parsed, err := entityalias.ParseLLMResponse(raw, sourceEntity.ID, allowed)
		if err != nil {
			return stored, err
		}
		for _, prop := range parsed {
			row, err := s.proposals.InsertPending(ctx, tenantID, repository.InsertEntityAliasProposalParams{
				Kind:           p.Kind,
				AliasKey:       prop.AliasKey,
				SourceEntityID: sourceEntity.ID,
				TargetEntityID: prop.TargetEntityID,
				SourceMemoryID: memoryID,
				Confidence:     prop.Confidence,
				Reason:         prop.Reason,
				Model:          s.model,
				Metadata:       map[string]interface{}{"proposed_by": "llm"},
			})
			if err != nil {
				return stored, err
			}
			if row != nil {
				stored = append(stored, *row)
			}
		}
	}
	if stored == nil {
		stored = []model.EntityAliasProposal{}
	}
	return stored, nil
}

func (s *EntityAliasProposalService) AcceptProposal(ctx context.Context, tenantID string, id int64) (*model.EntityAliasProposal, error) {
	prop, err := s.proposals.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if prop.Status != model.EntityAliasProposalStatusPending {
		return nil, fmt.Errorf("proposal is not pending")
	}
	targetEntity, err := s.registry.GetByID(ctx, tenantID, prop.TargetEntityID)
	if err != nil {
		return nil, err
	}
	if err := s.entities.ApplyAliasMerge(ctx, tenantID, prop.Kind, prop.AliasKey, targetEntity.CanonicalKey, model.EntityAliasSourceAliasProposal, prop.Confidence); err != nil {
		return nil, err
	}
	return s.proposals.UpdateStatus(ctx, tenantID, id, model.EntityAliasProposalStatusAccepted)
}

func (s *EntityAliasProposalService) RejectProposal(ctx context.Context, tenantID string, id int64) (*model.EntityAliasProposal, error) {
	return s.proposals.UpdateStatus(ctx, tenantID, id, model.EntityAliasProposalStatusRejected)
}
