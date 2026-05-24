package service

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
)

// stressRepo is a thread-safe mock repo for stress tests with optional latency.
type stressRepo struct {
	mu          sync.Mutex
	stored      map[string][]model.MemoryEntry // tenantID → entries
	latencyMax  time.Duration
	storeErr    error
	storeCalls  atomic.Int64
}

func newStressRepo(maxLatency time.Duration) *stressRepo {
	return &stressRepo{
		stored:     make(map[string][]model.MemoryEntry),
		latencyMax: maxLatency,
	}
}

func (r *stressRepo) sleep() {
	if r.latencyMax > 0 {
		time.Sleep(time.Duration(rand.Int63n(int64(r.latencyMax) + 1)))
	}
}

func (r *stressRepo) Store(_ context.Context, req model.StoreRequest, tenantID string) (int64, int, *int64, error) {
	r.sleep()
	if r.storeErr != nil {
		return 0, 0, nil, r.storeErr
	}
	r.storeCalls.Add(1)
	r.mu.Lock()
	id := int64(len(r.stored[tenantID]) + 1)
	r.stored[tenantID] = append(r.stored[tenantID], model.MemoryEntry{
		ID:       id,
		TenantID: tenantID,
		Path:     req.Path,
		Content:  req.Content,
	})
	r.mu.Unlock()
	return id, 1, nil, nil
}

func (r *stressRepo) Retrieve(_ context.Context, req model.RetrieveRequest, tenantID string, _ []float32) ([]model.MemoryEntry, error) {
	r.sleep()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.MemoryEntry
	for _, e := range r.stored[tenantID] {
		if req.PathPrefix == "" || e.Path == req.PathPrefix {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *stressRepo) GetByPath(_ context.Context, tenantID, path string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	r.sleep()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.stored[tenantID] {
		if e.Path == path {
			return &e, nil
		}
	}
	return nil, nil
}

func (r *stressRepo) GetHistoricalVersion(_ context.Context, _ string, _ string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	return nil, nil
}
func (r *stressRepo) ExportMemories(_ context.Context, _ string, _ string, _ int, _ bool) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (r *stressRepo) CompactPathHistory(_ context.Context, _ string, _ string, _ int) (int, error) {
	return 0, nil
}
func (r *stressRepo) UpdateImportance(_ context.Context, _, _ string, _ float64) error {
	return nil
}
func (r *stressRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return model.DedupModeNone, nil
}
func (r *stressRepo) FindCurrentByContentHash(context.Context, string, string) (*model.MemoryEntry, error) {
	return nil, nil
}
func (r *stressRepo) MergeCurrentMetadata(context.Context, string, string, map[string]interface{}, []string) (*model.MemoryEntry, error) {
	return nil, nil
}
func (r *stressRepo) UpsertDedupLink(context.Context, string, string, string) error { return nil }

func initStressRedis(t *testing.T) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())
}

// TestMemoryServiceStress_MultiTenantConcurrent spawns 20 tenants × 50
// goroutines each storing 10 memories: 10,000 total store calls. Verifies
// no panics, no data races, and that every store returns a non-zero ID.
func TestMemoryServiceStress_MultiTenantConcurrent(t *testing.T) {
	initStressRedis(t)

	const (
		tenants       = 20
		goroutines    = 50
		storesEach    = 10
	)

	repo := newStressRepo(0)
	svc := NewMemoryService(repo, nil)
	ctx := context.Background()

	var wg sync.WaitGroup
	var errors atomic.Int64
	var zeroIDs atomic.Int64

	for tenant := 0; tenant < tenants; tenant++ {
		tenantID := fmt.Sprintf("tenant-%02d", tenant)
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(tid string, g int) {
				defer wg.Done()
				for i := 0; i < storesEach; i++ {
					req := &model.StoreRequest{
						Path:    fmt.Sprintf("root.stress.%s.g%d.i%d", tid, g, i),
						Content: fmt.Sprintf("content for tenant=%s goroutine=%d item=%d", tid, g, i),
					}
					result, err := svc.Store(ctx, req, tid)
					if err != nil {
						errors.Add(1)
						return
					}
					if result.Entry.ID == 0 {
						zeroIDs.Add(1)
					}
				}
			}(tenantID, g)
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestMemoryServiceStress_MultiTenantConcurrent: deadlock")
	}

	if n := errors.Load(); n > 0 {
		t.Errorf("%d store calls returned errors", n)
	}
	if n := zeroIDs.Load(); n > 0 {
		t.Errorf("%d store results had zero ID", n)
	}

	expectedTotal := int64(tenants * goroutines * storesEach)
	if got := repo.storeCalls.Load(); got != expectedTotal {
		t.Errorf("storeCalls=%d, expected %d", got, expectedTotal)
	}
}

// TestMemoryServiceStress_SingleTenantHighVolume stores 2000 memories from
// 100 goroutines for a single tenant. Verifies correctness under high contention
// on the same tenant data structure.
func TestMemoryServiceStress_SingleTenantHighVolume(t *testing.T) {
	initStressRedis(t)

	const goroutines = 100
	const storesEach = 20

	repo := newStressRepo(0)
	svc := NewMemoryService(repo, nil)
	ctx := context.Background()
	tenantID := "stress-tenant-single"

	var wg sync.WaitGroup
	var errCount atomic.Int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < storesEach; i++ {
				req := &model.StoreRequest{
					Path:    fmt.Sprintf("root.bulk.g%d.i%d", g, i),
					Content: fmt.Sprintf("bulk content g=%d i=%d", g, i),
				}
				if _, err := svc.Store(ctx, req, tenantID); err != nil {
					errCount.Add(1)
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestMemoryServiceStress_SingleTenantHighVolume: deadlock")
	}

	if n := errCount.Load(); n > 0 {
		t.Errorf("%d store errors in high-volume single-tenant test", n)
	}
	if got := repo.storeCalls.Load(); got != int64(goroutines*storesEach) {
		t.Errorf("storeCalls=%d, expected %d", got, goroutines*storesEach)
	}
}

// TestMemoryServiceStress_RepoLatency verifies that 200 goroutines all
// complete when the repo has 0–5ms random latency and a 10s context deadline.
func TestMemoryServiceStress_RepoLatency(t *testing.T) {
	initStressRedis(t)

	repo := newStressRepo(5 * time.Millisecond)
	svc := NewMemoryService(repo, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const goroutines = 200
	var wg sync.WaitGroup
	var timeouts atomic.Int64
	var errs atomic.Int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			req := &model.StoreRequest{
				Path:    fmt.Sprintf("root.latency.g%d", g),
				Content: "latency test",
			}
			_, err := svc.Store(ctx, req, "latency-tenant")
			if err != nil {
				if ctx.Err() != nil {
					timeouts.Add(1)
				} else {
					errs.Add(1)
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("TestMemoryServiceStress_RepoLatency: deadlock")
	}

	if n := timeouts.Load(); n > 0 {
		t.Errorf("%d store calls hit context timeout — increase test deadline or reduce goroutines", n)
	}
	if n := errs.Load(); n > 0 {
		t.Errorf("%d unexpected store errors", n)
	}
}

// TestMemoryServiceStress_UniquePathsNoCollision verifies that storing N
// distinct paths from N goroutines results in exactly N entries (no collisions,
// no silent overwrites) when the repo is correct.
func TestMemoryServiceStress_UniquePathsNoCollision(t *testing.T) {
	initStressRedis(t)

	const goroutines = 500
	repo := newStressRepo(0)
	svc := NewMemoryService(repo, nil)
	ctx := context.Background()
	tenantID := "unique-paths-tenant"

	paths := make([]string, goroutines)
	for i := range paths {
		paths[i] = fmt.Sprintf("root.unique.path%04d", i)
	}

	var wg sync.WaitGroup
	var errs atomic.Int64
	for i, p := range paths {
		wg.Add(1)
		go func(path string, idx int) {
			defer wg.Done()
			req := &model.StoreRequest{
				Path:    path,
				Content: fmt.Sprintf("content for path %s", path),
			}
			result, err := svc.Store(ctx, req, tenantID)
			if err != nil {
				errs.Add(1)
				return
			}
			if result.Entry.Path != path {
				t.Errorf("returned path %q, expected %q", result.Entry.Path, path)
			}
		}(p, i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestMemoryServiceStress_UniquePathsNoCollision: deadlock")
	}

	if n := errs.Load(); n > 0 {
		t.Errorf("%d store errors", n)
	}
	if got := repo.storeCalls.Load(); got != goroutines {
		t.Errorf("storeCalls=%d, expected %d", got, goroutines)
	}
}

// TestMemoryServiceStress_ContentHashConcurrency calls model.ContentHash
// from 1000 goroutines simultaneously to verify it is goroutine-safe.
func TestMemoryServiceStress_ContentHashConcurrency(t *testing.T) {
	t.Parallel()
	const goroutines = 1000
	inputs := []string{
		"",
		"hello world",
		"the quick brown fox jumps over the lazy dog",
		"unicode: 日本語テスト",
		"a very long string " + string(make([]byte, 4096)),
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			input := inputs[g%len(inputs)]
			h1 := model.ContentHash(input)
			h2 := model.ContentHash(input)
			if h1 != h2 {
				t.Errorf("ContentHash is not deterministic for input len=%d", len(input))
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestMemoryServiceStress_ContentHashConcurrency: deadlock")
	}
}
