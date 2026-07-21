package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/linkproposal"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

const maxLinkProposalCandidates = 20

// LinkProposalService generates and reviews LLM link proposals.
type LinkProposalService struct {
	proposals linkProposalStore
	profiles  linkProposalProfileStore
	links     linkProposalLinkStore
	graph     linkProposalGraph
	llm       LLMCompleter
	model     string
	enabled   bool
}

type linkProposalStore interface {
	InsertPending(ctx context.Context, tenantID string, p repository.InsertLinkProposalParams) (*model.LinkProposal, error)
	GetByID(ctx context.Context, tenantID string, id int64) (*model.LinkProposal, error)
	List(ctx context.Context, tenantID, status string, sourceMemoryID int64, limit int) ([]model.LinkProposal, error)
	UpdateStatus(ctx context.Context, tenantID string, id int64, status string) (*model.LinkProposal, error)
}

type linkProposalProfileStore interface {
	GetCurrentMemoryByID(ctx context.Context, tenantID string, memoryID int64) (*model.MemoryEntry, error)
	MatchProfile(ctx context.Context, tenantID, path string) (*extraction.Profile, string, error)
}

type linkProposalLinkStore interface {
	Create(ctx context.Context, tenantID string, req model.CreateLinkRequest) (*model.MemoryLink, error)
}

type linkProposalGraph interface {
	IsAvailable(ctx context.Context) bool
	FindRelatedViaEntity(ctx context.Context, tenantID string, memoryID int64, cursor int64, limit int) (*graph.EntityMemoriesResult, error)
}

func NewLinkProposalService(
	proposals linkProposalStore,
	profiles linkProposalProfileStore,
	links linkProposalLinkStore,
	graphClient linkProposalGraph,
	llm LLMCompleter,
	cfg *config.Config,
) *LinkProposalService {
	s := &LinkProposalService{
		proposals: proposals,
		profiles:  profiles,
		links:     links,
		graph:     graphClient,
		llm:       llm,
		model:     "gpt-4o-mini",
	}
	if cfg != nil {
		s.enabled = cfg.LinkProposalsEnabled
		if m := strings.TrimSpace(cfg.ExtractionModel); m != "" {
			s.model = m
		} else if m := strings.TrimSpace(cfg.DistillationModel); m != "" {
			s.model = m
		}
	}
	return s
}

func (s *LinkProposalService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *LinkProposalService) List(ctx context.Context, tenantID, status string, sourceMemoryID int64, limit int) ([]model.LinkProposal, error) {
	return s.proposals.List(ctx, tenantID, status, sourceMemoryID, limit)
}

func (s *LinkProposalService) Get(ctx context.Context, tenantID string, id int64) (*model.LinkProposal, error) {
	return s.proposals.GetByID(ctx, tenantID, id)
}

// GenerateForMemory proposes links from a memory to entity-correlated candidates.
func (s *LinkProposalService) GenerateForMemory(ctx context.Context, tenantID string, memoryID int64) ([]model.LinkProposal, error) {
	if s == nil || s.proposals == nil || s.profiles == nil {
		return nil, fmt.Errorf("link proposal service not configured")
	}
	if !s.enabled {
		return nil, fmt.Errorf("link proposals are disabled")
	}
	if s.graph == nil || !s.graph.IsAvailable(ctx) {
		return nil, fmt.Errorf("cognitive graph not available")
	}
	if s.llm == nil || !s.llm.IsConfigured() {
		return nil, fmt.Errorf("LLM not configured")
	}

	entry, err := s.profiles.GetCurrentMemoryByID(ctx, tenantID, memoryID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("memory not found")
	}
	sourceRec, ok := extraction.RecordFromMetadata(metadataAsMap(entry.Metadata))
	if !ok || sourceRec == nil || sourceRec.Status != "ok" {
		return nil, fmt.Errorf("memory has no successful extraction")
	}

	profile, _, err := s.profiles.MatchProfile(ctx, tenantID, entry.Path)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("no matching extraction profile")
	}

	related, err := s.graph.FindRelatedViaEntity(ctx, tenantID, memoryID, 0, maxLinkProposalCandidates)
	if err != nil {
		return nil, err
	}
	if len(related.Memories) == 0 {
		return []model.LinkProposal{}, nil
	}

	allowed := make(map[int64]struct{}, len(related.Memories))
	candidates := make([]linkproposal.Candidate, 0, len(related.Memories))
	for _, mem := range related.Memories {
		allowed[mem.ID] = struct{}{}
		candEntry, err := s.profiles.GetCurrentMemoryByID(ctx, tenantID, mem.ID)
		if err != nil || candEntry == nil {
			continue
		}
		candRec, _ := extraction.RecordFromMetadata(metadataAsMap(candEntry.Metadata))
		candidates = append(candidates, linkproposal.Candidate{
			MemoryID:   candEntry.ID,
			Path:       candEntry.Path,
			Content:    candEntry.Content,
			Extraction: candRec,
		})
	}
	if len(candidates) == 0 {
		return []model.LinkProposal{}, nil
	}

	systemPrompt := linkproposal.BuildSystemPrompt(profile)
	userMsg := linkproposal.BuildUserMessage(entry, sourceRec, candidates)
	raw, err := s.llm.Complete(ctx, systemPrompt, []string{userMsg})
	if err != nil {
		return nil, fmt.Errorf("llm link proposals: %w", err)
	}
	parsed, err := linkproposal.ParseLLMResponse(raw, memoryID, allowed)
	if err != nil {
		return nil, err
	}

	fromPath := model.MemoryPathForID(memoryID)
	var stored []model.LinkProposal
	for _, p := range parsed {
		toPath := model.MemoryPathForID(p.ToMemoryID)
		row, err := s.proposals.InsertPending(ctx, tenantID, repository.InsertLinkProposalParams{
			SourceMemoryID: memoryID,
			FromMemoryID:   memoryID,
			ToMemoryID:     p.ToMemoryID,
			FromPath:       fromPath,
			ToPath:         toPath,
			LinkType:       p.LinkType,
			Confidence:     p.Confidence,
			Reason:         p.Reason,
			ProfileID:      profile.ProfileID,
			Model:          s.model,
			Metadata: map[string]interface{}{
				"proposed_by": "llm",
			},
		})
		if err != nil {
			return stored, err
		}
		if row != nil {
			stored = append(stored, *row)
		}
	}
	if stored == nil {
		stored = []model.LinkProposal{}
	}
	return stored, nil
}

// AcceptProposal materializes a pending proposal into memory_links.
func (s *LinkProposalService) AcceptProposal(ctx context.Context, tenantID string, id int64) (*model.MemoryLink, *model.LinkProposal, error) {
	prop, err := s.proposals.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, nil, err
	}
	if prop == nil {
		return nil, nil, fmt.Errorf("proposal not found")
	}
	if prop.Status != model.LinkProposalStatusPending {
		return nil, nil, fmt.Errorf("proposal is not pending")
	}

	meta := map[string]interface{}{
		"proposed_by":  "llm",
		"proposal_id":  prop.ID,
		"confidence":   prop.Confidence,
		"reason":       prop.Reason,
		"accepted":     true,
	}
	link, err := s.links.Create(ctx, tenantID, model.CreateLinkRequest{
		FromPath: prop.FromPath,
		ToPath:   prop.ToPath,
		LinkType: prop.LinkType,
		Metadata: meta,
	})
	if err != nil {
		return nil, nil, err
	}

	updated, err := s.proposals.UpdateStatus(ctx, tenantID, id, model.LinkProposalStatusAccepted)
	if err != nil {
		return link, nil, err
	}
	return link, updated, nil
}

// RejectProposal marks a pending proposal rejected without creating a link.
func (s *LinkProposalService) RejectProposal(ctx context.Context, tenantID string, id int64) (*model.LinkProposal, error) {
	prop, err := s.proposals.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if prop == nil {
		return nil, fmt.Errorf("proposal not found")
	}
	if prop.Status != model.LinkProposalStatusPending {
		return nil, fmt.Errorf("proposal is not pending")
	}
	return s.proposals.UpdateStatus(ctx, tenantID, id, model.LinkProposalStatusRejected)
}
