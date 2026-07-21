package service

import (
	"context"
	"errors"
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type fakeEntityAliasProposals struct {
	byID   map[int64]*model.EntityAliasProposal
	nextID int64
}

func (f *fakeEntityAliasProposals) InsertPending(_ context.Context, _ string, p repository.InsertEntityAliasProposalParams) (*model.EntityAliasProposal, error) {
	f.nextID++
	row := &model.EntityAliasProposal{
		ID: f.nextID, Kind: p.Kind, AliasKey: p.AliasKey, SourceEntityID: p.SourceEntityID,
		TargetEntityID: p.TargetEntityID, SourceMemoryID: p.SourceMemoryID,
		Status: model.EntityAliasProposalStatusPending, Confidence: p.Confidence, Reason: p.Reason,
	}
	if f.byID == nil {
		f.byID = map[int64]*model.EntityAliasProposal{}
	}
	f.byID[row.ID] = row
	return row, nil
}

func (f *fakeEntityAliasProposals) GetByID(_ context.Context, _ string, id int64) (*model.EntityAliasProposal, error) {
	return f.byID[id], nil
}

func (f *fakeEntityAliasProposals) List(_ context.Context, _, status string, _ int) ([]model.EntityAliasProposal, error) {
	var out []model.EntityAliasProposal
	for _, p := range f.byID {
		if status == "" || p.Status == status {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakeEntityAliasProposals) UpdateStatus(_ context.Context, _ string, id int64, status string) (*model.EntityAliasProposal, error) {
	p := f.byID[id]
	if p == nil {
		return nil, errors.New("not found")
	}
	p.Status = status
	return p, nil
}

type fakeEntityAliasRegistry struct {
	entities map[string]*model.EntityRegistry
}

func (f *fakeEntityAliasRegistry) key(kind, canonical string) string { return kind + ":" + canonical }

func (f *fakeEntityAliasRegistry) GetByCanonical(_ context.Context, _, kind, canonicalKey string) (*model.EntityRegistry, error) {
	if f.entities == nil {
		return nil, nil
	}
	return f.entities[f.key(kind, canonicalKey)], nil
}

func (f *fakeEntityAliasRegistry) GetByID(_ context.Context, _, id string) (*model.EntityRegistry, error) {
	for _, e := range f.entities {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeEntityAliasRegistry) ResolveCanonicalKey(_ context.Context, _, _, aliasKey string) (string, error) {
	return aliasKey, nil
}

func (f *fakeEntityAliasRegistry) ListEntities(_ context.Context, _, kind string, _ int) ([]model.EntityRegistry, error) {
	var out []model.EntityRegistry
	for _, e := range f.entities {
		if e.Kind == kind {
			out = append(out, *e)
		}
	}
	return out, nil
}

type fakeEntityAliasProfiles struct {
	entry   *model.MemoryEntry
	profile *extraction.Profile
}

func (f *fakeEntityAliasProfiles) GetCurrentMemoryByID(context.Context, string, int64) (*model.MemoryEntry, error) {
	return f.entry, nil
}

func (f *fakeEntityAliasProfiles) MatchProfile(context.Context, string, string) (*extraction.Profile, string, error) {
	return f.profile, "root.cti", nil
}

type stubAliasLLM struct {
	raw string
	err error
}

func (s *stubAliasLLM) Complete(context.Context, string, []string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.raw, nil
}

func (s *stubAliasLLM) IsConfigured() bool { return true }

func TestEntityAliasProposalService_Enabled(t *testing.T) {
	on := NewEntityAliasProposalService(nil, nil, nil, nil, nil, &config.Config{EntityAliasProposalsEnabled: true})
	if !on.Enabled() {
		t.Fatal("expected enabled")
	}
	off := NewEntityAliasProposalService(nil, nil, nil, nil, nil, &config.Config{})
	if off.Enabled() {
		t.Fatal("expected disabled")
	}
}

func TestEntityAliasProposalService_List(t *testing.T) {
	proposals := &fakeEntityAliasProposals{byID: map[int64]*model.EntityAliasProposal{
		1: {ID: 1, Status: model.EntityAliasProposalStatusPending},
	}}
	svc := NewEntityAliasProposalService(proposals, nil, nil, nil, nil, &config.Config{EntityAliasProposalsEnabled: true})
	out, err := svc.List(context.Background(), "t", model.EntityAliasProposalStatusPending, 10)
	if err != nil || len(out) != 1 {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestEntityAliasProposalService_GenerateForMemory(t *testing.T) {
	regStore := &fakeEntityAliasRegistry{entities: map[string]*model.EntityRegistry{
		"ThreatActor:FROZENLAKE": {ID: "src-1", Kind: "ThreatActor", CanonicalKey: "FROZENLAKE"},
		"ThreatActor:APT28":      {ID: "tgt-1", Kind: "ThreatActor", CanonicalKey: "APT28"},
	}}
	profiles := &fakeEntityAliasProfiles{
		entry: &model.MemoryEntry{
			ID: 7, Path: "root.cti.test",
			Metadata: map[string]interface{}{
				extraction.MetadataKey: map[string]interface{}{
					"status": "ok", "confidence": 0.9, "memory_id": float64(7), "memory_version": float64(1),
					"profile_id": "cti.v1", "profile_version": float64(1),
					"slots": map[string]interface{}{"actor": "FROZENLAKE"},
				},
			},
		},
		profile: &extraction.Profile{
			ProfileID: "cti.v1", Version: 1,
			EntityPromotion: map[string]extraction.EntityPromotion{
				"actor": {VertexLabel: "ThreatActor", Normalize: "trim"},
			},
		},
	}
	llm := &stubAliasLLM{raw: `{"proposals":[{"alias_key":"FROZENLAKE","target_entity_id":"tgt-1","confidence":0.88,"reason":"same actor"}]}`}
	entities := NewEntityRegistryService(&fakeEntityRegistryStore{})
	svc := NewEntityAliasProposalService(&fakeEntityAliasProposals{}, regStore, entities, profiles, llm, &config.Config{EntityAliasProposalsEnabled: true})

	out, err := svc.GenerateForMemory(context.Background(), "t", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].TargetEntityID != "tgt-1" {
		t.Fatalf("out=%+v", out)
	}
}

func TestEntityAliasProposalService_GenerateDisabled(t *testing.T) {
	svc := NewEntityAliasProposalService(nil, nil, nil, nil, nil, &config.Config{})
	_, err := svc.GenerateForMemory(context.Background(), "t", 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEntityAliasProposalService_AcceptReject(t *testing.T) {
	regStore := &fakeEntityRegistryStore{}
	entities := NewEntityRegistryService(regStore)
	proposals := &fakeEntityAliasProposals{byID: map[int64]*model.EntityAliasProposal{
		1: {
			ID: 1, Status: model.EntityAliasProposalStatusPending, Kind: "ThreatActor",
			AliasKey: "FROZENLAKE", TargetEntityID: "tgt-1",
		},
	}}
	regStore.entities = map[string]*model.EntityRegistry{
		"ThreatActor:APT28": {ID: "tgt-1", Kind: "ThreatActor", CanonicalKey: "APT28"},
	}
	svc := NewEntityAliasProposalService(proposals, regStore, entities, nil, nil, &config.Config{EntityAliasProposalsEnabled: true})

	accepted, err := svc.AcceptProposal(context.Background(), "t", 1)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != model.EntityAliasProposalStatusAccepted {
		t.Fatalf("status=%s", accepted.Status)
	}

	proposals.byID[2] = &model.EntityAliasProposal{ID: 2, Status: model.EntityAliasProposalStatusPending}
	rejected, err := svc.RejectProposal(context.Background(), "t", 2)
	if err != nil || rejected.Status != model.EntityAliasProposalStatusRejected {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
}
