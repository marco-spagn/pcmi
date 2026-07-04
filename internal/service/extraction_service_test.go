package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type fakeExtractionProfiles struct {
	entry   *model.MemoryEntry
	profile *extraction.Profile
}

func (f *fakeExtractionProfiles) ListProfiles(context.Context, string) ([]repository.ExtractionProfileRow, error) {
	return nil, nil
}

func (f *fakeExtractionProfiles) MatchProfile(context.Context, string, string) (*extraction.Profile, string, error) {
	return f.profile, "root", nil
}

func (f *fakeExtractionProfiles) UpsertProfile(context.Context, string, string, string, *extraction.Profile, bool) (*repository.ExtractionProfileRow, error) {
	return nil, nil
}

func (f *fakeExtractionProfiles) DeleteProfile(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *fakeExtractionProfiles) GetCurrentMemoryByID(context.Context, string, int64) (*model.MemoryEntry, error) {
	return f.entry, nil
}

type fakeExtractionMemoryRepo struct {
	meta map[string]interface{}
}

func (f *fakeExtractionMemoryRepo) Store(context.Context, model.StoreRequest, string) (int64, int, *int64, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) Retrieve(context.Context, model.RetrieveRequest, string, []float32) ([]model.MemoryEntry, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) GetHistoricalVersion(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) GetByPath(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) ExportMemories(context.Context, string, string, int, bool) ([]model.MemoryEntry, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) CompactPathHistory(context.Context, string, string, int) (int, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) UpdateImportance(context.Context, string, string, float64) error {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) FindCurrentByContentHash(context.Context, string, string) (*model.MemoryEntry, error) {
	panic("unused")
}
func (f *fakeExtractionMemoryRepo) UpsertDedupLink(context.Context, string, string, string) error {
	panic("unused")
}

func (f *fakeExtractionMemoryRepo) MergeCurrentMetadata(_ context.Context, _, path string, patch map[string]interface{}, _ []string) (*model.MemoryEntry, error) {
	if f.meta == nil {
		f.meta = map[string]interface{}{}
	}
	for k, v := range patch {
		f.meta[k] = v
	}
	return &model.MemoryEntry{Path: path, Metadata: f.meta}, nil
}

type stubLLM struct {
	raw string
	err error
	ok  bool
}

func (s stubLLM) Complete(context.Context, string, []string) (string, error) {
	return s.raw, s.err
}

func (s stubLLM) IsConfigured() bool { return s.ok }

func testSOCProfile() *extraction.Profile {
	p, err := extraction.ParseProfile(json.RawMessage(`{
		"profile_id":"soc.siem.v1","version":1,
		"required_slots":[
			{"name":"record_kind","type":"enum","values":["incident","alert"],"nullable":false},
			{"name":"disposition","type":"enum","values":["true_positive","false_positive","unknown"],"nullable":false},
			{"name":"src_ip","type":"ip","nullable":true}
		]
	}`))
	if err != nil {
		panic(err)
	}
	return p
}

func TestExtractionService_Enabled(t *testing.T) {
	s := NewExtractionService(nil, nil, nil, &config.Config{ExtractionEnabled: true}, nil, nil)
	if !s.Enabled() {
		t.Fatal("expected enabled")
	}
}

func TestExtractionService_ExtractMemory_Disabled(t *testing.T) {
	s := NewExtractionService(&fakeExtractionProfiles{}, &fakeExtractionMemoryRepo{}, stubLLM{ok: true}, &config.Config{}, nil, nil)
	_, err := s.ExtractMemory(context.Background(), "tid", 1)
	if err == nil || err.Error() != "extraction is disabled" {
		t.Fatalf("got %v", err)
	}
}

func TestExtractionService_ExtractPath_SkipsExistingOk(t *testing.T) {
	entry := &model.MemoryEntry{
		ID: 1, Version: 2, Path: "root.inc1", Content: "x",
		Metadata: map[string]interface{}{
			extraction.MetadataKey: map[string]interface{}{
				"status": "ok", "memory_id": float64(1), "memory_version": float64(2),
			},
		},
	}
	s := NewExtractionService(
		&fakeExtractionProfiles{entry: entry, profile: testSOCProfile()},
		&fakeExtractionMemoryRepo{},
		stubLLM{ok: true, raw: `{"confidence":0.9,"slots":{"record_kind":"incident","disposition":"unknown","src_ip":null}}`},
		&config.Config{ExtractionEnabled: true},
		nil,
		nil,
	)
	if err := s.ExtractPath(context.Background(), "tid", "root.inc1", 1, 2); err != nil {
		t.Fatal(err)
	}
}

func TestExtractionService_ExtractMemory_Success(t *testing.T) {
	entry := &model.MemoryEntry{ID: 5, Version: 1, Path: "root.inc1", Content: "incident from 10.0.0.1"}
	mem := &fakeExtractionMemoryRepo{}
	s := NewExtractionService(
		&fakeExtractionProfiles{entry: entry, profile: testSOCProfile()},
		mem,
		stubLLM{ok: true, raw: `{"confidence":0.91,"slots":{"record_kind":"incident","disposition":"unknown","src_ip":"10.0.0.1"}}`},
		&config.Config{ExtractionEnabled: true, ExtractionModel: "test-model"},
		nil,
		nil,
	)
	rec, err := s.ExtractMemory(context.Background(), "tid", 5)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "ok" || rec.Model != "test-model" {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestExtractionService_ExtractMemory_LLMNotConfigured(t *testing.T) {
	entry := &model.MemoryEntry{ID: 5, Version: 1, Path: "root.inc1", Content: "body"}
	s := NewExtractionService(
		&fakeExtractionProfiles{entry: entry, profile: testSOCProfile()},
		&fakeExtractionMemoryRepo{},
		stubLLM{ok: false},
		&config.Config{ExtractionEnabled: true},
		nil,
		nil,
	)
	rec, err := s.ExtractMemory(context.Background(), "tid", 5)
	if err == nil || rec == nil || rec.Error != "LLM not configured" {
		t.Fatalf("got rec=%+v err=%v", rec, err)
	}
}

func TestExtractionService_ExtractMemory_ValidationFailed(t *testing.T) {
	entry := &model.MemoryEntry{ID: 5, Version: 1, Path: "root.inc1", Content: "body"}
	s := NewExtractionService(
		&fakeExtractionProfiles{entry: entry, profile: testSOCProfile()},
		&fakeExtractionMemoryRepo{},
		stubLLM{ok: true, raw: `{"confidence":0.5,"slots":{"record_kind":"incident","disposition":null,"src_ip":null}}`},
		&config.Config{ExtractionEnabled: true},
		nil,
		nil,
	)
	rec, err := s.ExtractMemory(context.Background(), "tid", 5)
	if err == nil || rec == nil || rec.Status != "validation_failed" {
		t.Fatalf("got rec=%+v err=%v", rec, err)
	}
}

func TestExtractionService_ExtractMemory_LLMError(t *testing.T) {
	entry := &model.MemoryEntry{ID: 5, Version: 1, Path: "root.inc1", Content: "body"}
	s := NewExtractionService(
		&fakeExtractionProfiles{entry: entry, profile: testSOCProfile()},
		&fakeExtractionMemoryRepo{},
		stubLLM{ok: true, err: errors.New("timeout")},
		&config.Config{ExtractionEnabled: true},
		nil,
		nil,
	)
	rec, err := s.ExtractMemory(context.Background(), "tid", 5)
	if err == nil || rec == nil || rec.Status != "llm_failed" {
		t.Fatalf("got rec=%+v err=%v", rec, err)
	}
}

func TestExtractionService_GetExtraction(t *testing.T) {
	entry := &model.MemoryEntry{
		ID: 3, Path: "root.a", Version: 1,
		Metadata: map[string]interface{}{
			extraction.MetadataKey: map[string]interface{}{"status": "ok", "memory_id": float64(3), "memory_version": float64(1)},
		},
	}
	s := NewExtractionService(&fakeExtractionProfiles{entry: entry}, nil, nil, nil, nil, nil)
	rec, gotEntry, err := s.GetExtraction(context.Background(), "tid", 3)
	if err != nil || rec == nil || gotEntry == nil || rec.Status != "ok" {
		t.Fatalf("got rec=%+v entry=%+v err=%v", rec, gotEntry, err)
	}
}

func TestExtractionService_ExtractPath_SkipDistilled(t *testing.T) {
	s := NewExtractionService(nil, nil, nil, &config.Config{ExtractionEnabled: true}, nil, nil)
	if err := s.ExtractPath(context.Background(), "tid", "root.distilled.x", 1, 1); err != nil {
		t.Fatal(err)
	}
}

func TestExistingOkExtraction(t *testing.T) {
	entry := &model.MemoryEntry{
		ID: 1, Version: 1,
		Metadata: map[string]interface{}{
			extraction.MetadataKey: map[string]interface{}{
				"status": "ok", "memory_id": float64(1), "memory_version": float64(1),
			},
		},
	}
	rec, ok := existingOkExtraction(entry)
	if !ok || rec == nil {
		t.Fatal("expected ok extraction")
	}
}
