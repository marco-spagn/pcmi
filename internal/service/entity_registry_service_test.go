package service

import (
	"context"
	"errors"
	"testing"

	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
)

type fakeEntityRegistryStore struct {
	entities   map[string]*model.EntityRegistry
	aliases    []model.EntityAlias
	snapshots  []model.EntitySnapshot
	resolveKey string
	mergeCalls int
}

func (f *fakeEntityRegistryStore) key(kind, canonical string) string { return kind + ":" + canonical }

func (f *fakeEntityRegistryStore) ResolveCanonicalKey(_ context.Context, _, _, _ string) (string, error) {
	if f.resolveKey != "" {
		return f.resolveKey, nil
	}
	return "", nil
}

func (f *fakeEntityRegistryStore) UpsertCanonical(_ context.Context, _, kind, canonicalKey, displayName string, _ map[string]interface{}) (*model.EntityRegistry, error) {
	if f.entities == nil {
		f.entities = map[string]*model.EntityRegistry{}
	}
	k := f.key(kind, canonicalKey)
	if e, ok := f.entities[k]; ok {
		return e, nil
	}
	e := &model.EntityRegistry{ID: "ent-" + canonicalKey, Kind: kind, CanonicalKey: canonicalKey, DisplayName: displayName}
	f.entities[k] = e
	return e, nil
}

func (f *fakeEntityRegistryStore) UpsertActiveAlias(_ context.Context, _, entityID, kind, aliasKey, source string, confidence float64, _ map[string]interface{}) error {
	f.aliases = append(f.aliases, model.EntityAlias{EntityID: entityID, Kind: kind, AliasKey: aliasKey, Source: source, Confidence: confidence})
	return nil
}

func (f *fakeEntityRegistryStore) InsertSnapshot(_ context.Context, _, entityID string, memoryID int64, memoryVersion int, profileID, slot, rawKey string, _ map[string]interface{}, confidence float64) error {
	f.snapshots = append(f.snapshots, model.EntitySnapshot{
		EntityID: entityID, MemoryID: memoryID, MemoryVersion: memoryVersion,
		ProfileID: profileID, Slot: slot, RawKey: rawKey, Confidence: confidence,
	})
	return nil
}

func (f *fakeEntityRegistryStore) ListEntities(_ context.Context, _, kind string, _ int) ([]model.EntityRegistry, error) {
	var out []model.EntityRegistry
	for _, e := range f.entities {
		if e.Kind == kind {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (f *fakeEntityRegistryStore) GetByCanonical(_ context.Context, _, kind, canonicalKey string) (*model.EntityRegistry, error) {
	if e, ok := f.entities[f.key(kind, canonicalKey)]; ok {
		return e, nil
	}
	return nil, nil
}

func (f *fakeEntityRegistryStore) GetByID(_ context.Context, _, id string) (*model.EntityRegistry, error) {
	for _, e := range f.entities {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeEntityRegistryStore) ListActiveAliases(_ context.Context, _, entityID string) ([]model.EntityAlias, error) {
	var out []model.EntityAlias
	for _, a := range f.aliases {
		if a.EntityID == entityID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeEntityRegistryStore) ListSnapshots(_ context.Context, _, entityID string, _ int) ([]model.EntitySnapshot, error) {
	var out []model.EntitySnapshot
	for _, s := range f.snapshots {
		if s.EntityID == entityID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeEntityRegistryStore) ExpandEntityKeys(_ context.Context, _, _, key string) ([]string, error) {
	return []string{key, key + "-alias"}, nil
}

func (f *fakeEntityRegistryStore) MergeEntityAliasInGraph(_ context.Context, _, _, _, _ string) error {
	f.mergeCalls++
	return nil
}

func TestEntityRegistryService_SyncFromExtraction(t *testing.T) {
	store := &fakeEntityRegistryStore{resolveKey: "APT28"}
	svc := NewEntityRegistryService(store)
	profile := &extraction.Profile{
		ProfileID: "cti.v1", Version: 1,
		EntityPromotion: map[string]extraction.EntityPromotion{
			"actor": {VertexLabel: "ThreatActor", Normalize: "alias_table"},
		},
	}
	rec := &extraction.Record{
		Status: "ok", Confidence: 0.9,
		Slots: map[string]interface{}{"actor": "FROZENLAKE"},
	}
	entry := &model.MemoryEntry{ID: 42, Version: 2, Path: "root.cti.test"}

	mentions, err := svc.SyncFromExtraction(context.Background(), "tenant-1", entry, profile, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(mentions) != 1 || mentions[0].Key != "APT28" {
		t.Fatalf("mentions: %+v", mentions)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("expected snapshot, got %d", len(store.snapshots))
	}
}

func TestEntityRegistryService_SyncFromExtractionSkipsInvalid(t *testing.T) {
	svc := NewEntityRegistryService(&fakeEntityRegistryStore{})
	if mentions, err := svc.SyncFromExtraction(context.Background(), "t", nil, nil, nil); err != nil || mentions != nil {
		t.Fatalf("got mentions=%v err=%v", mentions, err)
	}
}

func TestEntityRegistryService_GetEntity(t *testing.T) {
	store := &fakeEntityRegistryStore{}
	svc := NewEntityRegistryService(store)
	_, _ = store.UpsertCanonical(context.Background(), "t", "ThreatActor", "APT28", "APT28", nil)

	entity, aliases, snaps, err := svc.GetEntity(context.Background(), "t", "ThreatActor", "APT28")
	if err != nil {
		t.Fatal(err)
	}
	if entity == nil || entity.CanonicalKey != "APT28" {
		t.Fatalf("entity: %+v", entity)
	}
	if len(aliases) != 0 || len(snaps) != 0 {
		t.Fatal("expected empty aliases and snapshots")
	}
}

func TestEntityRegistryService_GetEntityNotFound(t *testing.T) {
	svc := NewEntityRegistryService(&fakeEntityRegistryStore{})
	_, _, _, err := svc.GetEntity(context.Background(), "t", "ThreatActor", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEntityRegistryService_AddManualAlias(t *testing.T) {
	store := &fakeEntityRegistryStore{}
	svc := NewEntityRegistryService(store)
	entity, err := svc.AddManualAlias(context.Background(), "t", "ThreatActor", "APT28", "FROZENLAKE", 0.85)
	if err != nil {
		t.Fatal(err)
	}
	if entity.CanonicalKey != "APT28" {
		t.Fatalf("entity: %+v", entity)
	}
	if store.mergeCalls != 1 {
		t.Fatalf("mergeCalls=%d", store.mergeCalls)
	}
}

func TestEntityRegistryService_AddManualAliasValidation(t *testing.T) {
	svc := NewEntityRegistryService(&fakeEntityRegistryStore{})
	_, err := svc.AddManualAlias(context.Background(), "t", "ThreatActor", "", "x", 0.5)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestEntityRegistryService_ListEntitiesNotConfigured(t *testing.T) {
	var svc *EntityRegistryService
	_, err := svc.ListEntities(context.Background(), "t", "ThreatActor", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEntityRegistryService_ExpandEntityKeysFallback(t *testing.T) {
	var svc *EntityRegistryService
	keys, err := svc.ExpandEntityKeys(context.Background(), "t", "ThreatActor", "APT28")
	if err != nil || len(keys) != 1 || keys[0] != "APT28" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
}

func TestEntityRegistryService_ApplyAliasMerge(t *testing.T) {
	store := &fakeEntityRegistryStore{}
	svc := NewEntityRegistryService(store)
	if err := svc.ApplyAliasMerge(context.Background(), "t", "ThreatActor", "FROZENLAKE", "APT28", model.EntityAliasSourceManual, 0.9); err != nil {
		t.Fatal(err)
	}
	if store.mergeCalls != 1 {
		t.Fatalf("mergeCalls=%d", store.mergeCalls)
	}
}
