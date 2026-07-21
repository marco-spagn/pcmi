package service

import (
	"context"
	"errors"
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type fakeLinkProposals struct {
	byID   map[int64]*model.LinkProposal
	insert []*model.LinkProposal
}

func (f *fakeLinkProposals) InsertPending(_ context.Context, _ string, p repository.InsertLinkProposalParams) (*model.LinkProposal, error) {
	row := &model.LinkProposal{
		ID: int64(len(f.insert) + 1), SourceMemoryID: p.SourceMemoryID,
		FromMemoryID: p.FromMemoryID, ToMemoryID: p.ToMemoryID,
		FromPath: p.FromPath, ToPath: p.ToPath, LinkType: p.LinkType,
		Status: model.LinkProposalStatusPending, Confidence: p.Confidence, Reason: p.Reason,
	}
	f.insert = append(f.insert, row)
	if f.byID == nil {
		f.byID = map[int64]*model.LinkProposal{}
	}
	f.byID[row.ID] = row
	return row, nil
}

func (f *fakeLinkProposals) GetByID(_ context.Context, _ string, id int64) (*model.LinkProposal, error) {
	return f.byID[id], nil
}

func (f *fakeLinkProposals) List(_ context.Context, _, status string, _ int64, _ int) ([]model.LinkProposal, error) {
	var out []model.LinkProposal
	for _, p := range f.byID {
		if p.Status == status {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakeLinkProposals) UpdateStatus(_ context.Context, _ string, id int64, status string) (*model.LinkProposal, error) {
	p := f.byID[id]
	if p == nil {
		return nil, nil
	}
	p.Status = status
	return p, nil
}

type fakeLinkProposalProfiles struct {
	entry   *model.MemoryEntry
	profile *extraction.Profile
}

func (f *fakeLinkProposalProfiles) GetCurrentMemoryByID(context.Context, string, int64) (*model.MemoryEntry, error) {
	return f.entry, nil
}

func (f *fakeLinkProposalProfiles) MatchProfile(context.Context, string, string) (*extraction.Profile, string, error) {
	return f.profile, "root", nil
}

type fakeLinkProposalLinks struct {
	created *model.MemoryLink
}

func (f *fakeLinkProposalLinks) Create(_ context.Context, _ string, req model.CreateLinkRequest) (*model.MemoryLink, error) {
	f.created = &model.MemoryLink{FromPath: req.FromPath, ToPath: req.ToPath, LinkType: req.LinkType, Metadata: req.Metadata}
	return f.created, nil
}

type fakeLinkProposalGraph struct {
	available bool
	related   *graph.EntityMemoriesResult
}

func (f *fakeLinkProposalGraph) IsAvailable(context.Context) bool { return f.available }

func (f *fakeLinkProposalGraph) FindRelatedViaEntity(context.Context, string, int64, int64, int) (*graph.EntityMemoriesResult, error) {
	return f.related, nil
}

func TestLinkProposalService_Enabled(t *testing.T) {
	s := NewLinkProposalService(nil, nil, nil, nil, nil, &config.Config{LinkProposalsEnabled: true})
	if !s.Enabled() {
		t.Fatal("expected enabled")
	}
}

func TestLinkProposalService_AcceptProposal(t *testing.T) {
	store := &fakeLinkProposals{byID: map[int64]*model.LinkProposal{
		1: {ID: 1, Status: model.LinkProposalStatusPending, FromPath: "memory.1", ToPath: "memory.2", LinkType: "related"},
	}}
	links := &fakeLinkProposalLinks{}
	s := NewLinkProposalService(store, nil, links, nil, nil, &config.Config{LinkProposalsEnabled: true})
	link, prop, err := s.AcceptProposal(context.Background(), "tid", 1)
	if err != nil || link == nil || prop == nil || prop.Status != model.LinkProposalStatusAccepted {
		t.Fatalf("link=%+v prop=%+v err=%v", link, prop, err)
	}
}

func TestLinkProposalService_RejectProposal(t *testing.T) {
	store := &fakeLinkProposals{byID: map[int64]*model.LinkProposal{
		2: {ID: 2, Status: model.LinkProposalStatusPending},
	}}
	s := NewLinkProposalService(store, nil, nil, nil, nil, &config.Config{LinkProposalsEnabled: true})
	prop, err := s.RejectProposal(context.Background(), "tid", 2)
	if err != nil || prop == nil || prop.Status != model.LinkProposalStatusRejected {
		t.Fatalf("prop=%+v err=%v", prop, err)
	}
}

func TestLinkProposalService_GenerateForMemory_Disabled(t *testing.T) {
	s := NewLinkProposalService(&fakeLinkProposals{}, &fakeLinkProposalProfiles{}, nil, nil, nil, &config.Config{})
	_, err := s.GenerateForMemory(context.Background(), "tid", 1)
	if err == nil || err.Error() != "link proposals are disabled" {
		t.Fatalf("got %v", err)
	}
}

func TestLinkProposalService_GenerateForMemory_NoGraph(t *testing.T) {
	entry := &model.MemoryEntry{ID: 1, Path: "root.a", Content: "x", Metadata: map[string]interface{}{
		extraction.MetadataKey: map[string]interface{}{"status": "ok", "memory_id": float64(1), "memory_version": float64(1)},
	}}
	s := NewLinkProposalService(&fakeLinkProposals{}, &fakeLinkProposalProfiles{entry: entry, profile: testSOCProfile()}, nil,
		&fakeLinkProposalGraph{available: false}, stubLLM{ok: true}, &config.Config{LinkProposalsEnabled: true})
	_, err := s.GenerateForMemory(context.Background(), "tid", 1)
	if err == nil || err.Error() != "cognitive graph not available" {
		t.Fatalf("got %v", err)
	}
}

func TestLinkProposalService_GenerateForMemory_Success(t *testing.T) {
	source := &model.MemoryEntry{
		ID: 1, Version: 1, Path: "root.a", Content: "incident",
		Metadata: map[string]interface{}{
			extraction.MetadataKey: map[string]interface{}{
				"status": "ok", "memory_id": float64(1), "memory_version": float64(1),
				"slots": map[string]interface{}{"record_kind": "incident", "disposition": "unknown", "src_ip": "10.0.0.2"},
			},
		},
	}
	candidate := &model.MemoryEntry{
		ID: 2, Version: 1, Path: "root.b", Content: "related incident",
		Metadata: map[string]interface{}{
			extraction.MetadataKey: map[string]interface{}{
				"status": "ok", "memory_id": float64(2), "memory_version": float64(1),
				"slots": map[string]interface{}{"record_kind": "incident", "disposition": "unknown", "src_ip": "10.0.0.2"},
			},
		},
	}
	profilesWithCandidates := &linkProposalProfilesByID{
		byID: map[int64]*model.MemoryEntry{1: source, 2: candidate},
		profile: testSOCProfile(),
	}
	store := &fakeLinkProposals{byID: map[int64]*model.LinkProposal{}}
	g := &fakeLinkProposalGraph{
		available: true,
		related:   &graph.EntityMemoriesResult{Memories: []graph.EntityMemory{{ID: 2}}},
	}
	s := NewLinkProposalService(store, profilesWithCandidates, nil, g,
		stubLLM{ok: true, raw: `{"proposals":[{"to_memory_id":2,"link_type":"related","confidence":0.8,"reason":"shared src_ip"}]}`},
		&config.Config{LinkProposalsEnabled: true},
	)
	rows, err := s.GenerateForMemory(context.Background(), "tid", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ToMemoryID != 2 {
		t.Fatalf("got %+v", rows)
	}
}

type linkProposalProfilesByID struct {
	byID    map[int64]*model.MemoryEntry
	profile *extraction.Profile
}

func (f *linkProposalProfilesByID) GetCurrentMemoryByID(_ context.Context, _ string, memoryID int64) (*model.MemoryEntry, error) {
	return f.byID[memoryID], nil
}

func (f *linkProposalProfilesByID) MatchProfile(context.Context, string, string) (*extraction.Profile, string, error) {
	return f.profile, "root", nil
}

func TestLinkProposalService_AcceptProposal_NotPending(t *testing.T) {
	store := &fakeLinkProposals{byID: map[int64]*model.LinkProposal{
		1: {ID: 1, Status: model.LinkProposalStatusAccepted},
	}}
	s := NewLinkProposalService(store, nil, nil, nil, nil, &config.Config{LinkProposalsEnabled: true})
	_, _, err := s.AcceptProposal(context.Background(), "tid", 1)
	if err == nil || err.Error() != "proposal is not pending" {
		t.Fatalf("got %v", err)
	}
}

func TestLinkProposalService_GenerateForMemory_LLMError(t *testing.T) {
	entry := &model.MemoryEntry{ID: 1, Path: "root.a", Content: "x", Metadata: map[string]interface{}{
		extraction.MetadataKey: map[string]interface{}{"status": "ok", "memory_id": float64(1), "memory_version": float64(1)},
	}}
	s := NewLinkProposalService(nil, &fakeLinkProposalProfiles{entry: entry, profile: testSOCProfile()}, nil,
		&fakeLinkProposalGraph{available: true, related: &graph.EntityMemoriesResult{Memories: []graph.EntityMemory{{ID: 2}}}},
		stubLLM{ok: true, err: errors.New("boom")}, &config.Config{LinkProposalsEnabled: true})
	_, err := s.GenerateForMemory(context.Background(), "tid", 1)
	if err == nil {
		t.Fatal("expected error")
	}
}
