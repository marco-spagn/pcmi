package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

type dedupMockRepo struct {
	mu           sync.Mutex
	entries      map[string]*model.MemoryEntry // path -> current
	links        []struct{ from, to string }
	storeCalls   int
	mergeCalls   int
	tenantMode   model.DedupMode
	findByHashFn func(hash string) *model.MemoryEntry
}

func (d *dedupMockRepo) seed(path, content string, id int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries == nil {
		d.entries = map[string]*model.MemoryEntry{}
	}
	h := model.ContentHash(content)
	d.entries[path] = &model.MemoryEntry{
		ID: id, Path: path, Content: content, Version: 1,
		Metadata: map[string]interface{}{},
	}
	_ = h
}

func (d *dedupMockRepo) currentByHash(hash string) *model.MemoryEntry {
	if d.findByHashFn != nil {
		return d.findByHashFn(hash)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if model.ContentHash(e.Content) == hash {
			cp := *e
			return &cp
		}
	}
	return nil
}

func (d *dedupMockRepo) Store(_ context.Context, req model.StoreRequest, _ string) (int64, int, *int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries == nil {
		d.entries = map[string]*model.MemoryEntry{}
	}
	d.storeCalls++
	id := int64(100 + d.storeCalls)
	d.entries[req.Path] = &model.MemoryEntry{
		ID: id, Path: req.Path, Content: req.Content, Version: 1,
		Metadata: req.Metadata, Tags: req.Tags,
	}
	return id, 1, nil, nil
}

func (d *dedupMockRepo) Retrieve(context.Context, model.RetrieveRequest, string, []float32) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (d *dedupMockRepo) GetHistoricalVersion(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (d *dedupMockRepo) GetByPath(_ context.Context, _ string, path string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.entries[path]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, errNotFound
}
func (d *dedupMockRepo) ExportMemories(context.Context, string, string, int, bool) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (d *dedupMockRepo) CompactPathHistory(context.Context, string, string, int) (int, error) {
	return 0, nil
}
func (d *dedupMockRepo) UpdateImportance(context.Context, string, string, float64) error {
	return nil
}
func (d *dedupMockRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return d.tenantMode, nil
}
func (d *dedupMockRepo) FindCurrentByContentHash(_ context.Context, _ string, hash string) (*model.MemoryEntry, error) {
	return d.currentByHash(hash), nil
}
func (d *dedupMockRepo) MergeCurrentMetadata(_ context.Context, _ string, path string, metadata map[string]interface{}, tags []string) (*model.MemoryEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mergeCalls++
	e := d.entries[path]
	if e == nil {
		return nil, errNotFound
	}
	meta, _ := e.Metadata.(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
		e.Metadata = meta
	}
	for k, v := range metadata {
		meta[k] = v
	}
	if len(tags) > 0 {
		e.Tags = append([]string{}, tags...)
	}
	cp := *e
	return &cp, nil
}
func (d *dedupMockRepo) UpsertDedupLink(_ context.Context, _, fromPath, toPath string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.links = append(d.links, struct{ from, to string }{fromPath, toPath})
	return nil
}

var errNotFound = errors.New("not found")

func dedupSvc(repo *dedupMockRepo, mode model.DedupMode) *MemoryService {
	return NewMemoryService(repo, nil, mode)
}

func TestDedup_SameContentSamePath_Skip_ReturnsExisting(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.note", "same body", 42)
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.note", Content: "same body",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionSkipped || res.Entry.ID != 42 {
		t.Fatalf("got action=%q id=%d", res.Action, res.Entry.ID)
	}
	if repo.storeCalls != 0 {
		t.Fatalf("expected no store, calls=%d", repo.storeCalls)
	}
}

func TestDedup_SameContentDifferentPath_Link_CreatesLink(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.canonical", "shared text", 7)
	svc := dedupSvc(repo, model.DedupModeLink)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.alias", Content: "shared text",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionLinked || res.Entry.Path != "root.canonical" {
		t.Fatalf("got action=%q path=%s", res.Action, res.Entry.Path)
	}
	if len(repo.links) != 1 || repo.links[0].from != "root.alias" || repo.links[0].to != "root.canonical" {
		t.Fatalf("links=%+v", repo.links)
	}
	if repo.storeCalls != 0 {
		t.Fatalf("expected no store")
	}
}

func TestDedup_DifferentContent_AlwaysStores(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.a", "alpha", 1)
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.b", Content: "beta",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionStored || repo.storeCalls != 1 {
		t.Fatalf("action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
}

func TestDedup_Merge_UpdatesMetadataOnly(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.doc", "unchanged", 5)
	svc := dedupSvc(repo, model.DedupModeMerge)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.doc", Content: "unchanged",
		Metadata: map[string]interface{}{"source": "agent"},
		Tags:     []string{"v2"},
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionMerged || repo.mergeCalls != 1 || repo.storeCalls != 0 {
		t.Fatalf("action=%q merge=%d store=%d", res.Action, repo.mergeCalls, repo.storeCalls)
	}
	meta, _ := res.Entry.Metadata.(map[string]interface{})
	if meta["source"] != "agent" {
		t.Fatalf("metadata=%v", res.Entry.Metadata)
	}
}

func TestDedup_NormalizationCaseInsensitive(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.x", "Hello", 9)
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.x", Content: "  hello  ",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionSkipped {
		t.Fatalf("expected skip, got %q", res.Action)
	}
}

func TestDedup_NormalizationUnicode(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.u", "caf\u00e9", 3)
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.u", Content: "cafe\u0301",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionSkipped {
		t.Fatalf("expected skip, got %q", res.Action)
	}
}

func TestDedup_EmptyContentSamePath_Skip(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.empty", "", 11)
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.empty", Content: "  \t  ",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionSkipped || repo.storeCalls != 0 {
		t.Fatalf("action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
}

func TestDedup_ModeNone_AlwaysStoresEvenDuplicate(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{tenantMode: model.DedupModeSkip}
	svc := dedupSvc(repo, model.DedupModeNone)
	repo.seed("root.dup", "same", 1)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.dup", Content: "same", DedupMode: "none",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionStored || repo.storeCalls != 1 {
		t.Fatalf("none mode should store: action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
}

func TestDedup_ModeSkip_SkipsDuplicate(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{tenantMode: model.DedupModeNone}
	svc := dedupSvc(repo, model.DedupModeNone)
	repo.seed("root.dup", "same", 1)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.dup", Content: "same", DedupMode: "skip",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionSkipped || repo.storeCalls != 0 {
		t.Fatalf("skip override should skip: action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
}

func TestDedup_RequestModeOverridesTenantDefault(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{tenantMode: model.DedupModeSkip}
	svc := dedupSvc(repo, model.DedupModeSkip)
	repo.seed("root.a", "body", 2)
	repo.seed("root.b", "body", 3)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.new", Content: "body", DedupMode: "link",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionLinked {
		t.Fatalf("link override expected, got %q", res.Action)
	}
}

func TestDedup_LinkMode_NoExistingHash_Stores(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	svc := dedupSvc(repo, model.DedupModeLink)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.new", Content: "fresh content",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionStored || repo.storeCalls != 1 {
		t.Fatalf("expected store when no hash match: action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
	if len(repo.links) != 0 {
		t.Fatalf("expected no links, got %+v", repo.links)
	}
}

func TestDedup_Merge_ConflictingMetadata_Overwrites(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.doc", "text", 5)
	repo.entries["root.doc"].Metadata = map[string]interface{}{"a": "old", "keep": true}
	svc := dedupSvc(repo, model.DedupModeMerge)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.doc", Content: "text",
		Metadata: map[string]interface{}{"a": "new", "b": 2},
		Tags:     []string{"t1"},
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionMerged {
		t.Fatalf("expected merge, got %q", res.Action)
	}
	meta, _ := res.Entry.Metadata.(map[string]interface{})
	if meta["a"] != "new" || meta["b"] != 2 || meta["keep"] != true {
		t.Fatalf("metadata=%v", meta)
	}
}

func TestDedup_MergeDifferentPath_Stores(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.a", "shared", 1)
	svc := dedupSvc(repo, model.DedupModeMerge)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.b", Content: "shared",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionStored || repo.storeCalls != 1 || repo.mergeCalls != 0 {
		t.Fatalf("merge mode different path should store: action=%q store=%d merge=%d", res.Action, repo.storeCalls, repo.mergeCalls)
	}
}

func TestDedup_HashMismatchDifferentContent_Stores(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.findByHashFn = func(_ string) *model.MemoryEntry {
		return &model.MemoryEntry{ID: 99, Path: "root.stale", Content: "different body", Version: 1}
	}
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.x", Content: "incoming body",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionStored || repo.storeCalls != 1 {
		t.Fatalf("hash mismatch must store: action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
}

func TestDedup_SkipDifferentPath_Stores(t *testing.T) {
	t.Parallel()
	repo := &dedupMockRepo{}
	repo.seed("root.a", "shared", 1)
	svc := dedupSvc(repo, model.DedupModeSkip)

	res, err := svc.Store(context.Background(), &model.StoreRequest{
		Path: "root.b", Content: "shared",
	}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != model.StoreActionStored || repo.storeCalls != 1 {
		t.Fatalf("skip mode different path should store: action=%q storeCalls=%d", res.Action, repo.storeCalls)
	}
}
