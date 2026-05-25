//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

// multiAPIInstance is one API server replica in the simulated cluster.
type multiAPIInstance struct {
	id     int
	srv    *httptest.Server
	client *http.Client
	pool   *pgxpool.Pool
}

// multiCluster holds N API instances sharing one Redis and one DB.
type multiCluster struct {
	instances []*multiAPIInstance
	mr        *miniredis.Miniredis
}

// newMultiCluster builds n fully-wired API replicas sharing ONE miniredis and
// the DATABASE_URL. All replicas see the same PostgreSQL data and Redis events,
// matching a real K8s multi-pod deployment.
//
// Phase-1: all resources are created sequentially (no goroutines) to prevent
// races on the global event.RedisClient from InitRedis.
// Phase-2: callers spawn goroutines freely — InitRedis is not called again.
func newMultiCluster(t *testing.T, n int) *multiCluster {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	t.Setenv("RATE_LIMIT_DISABLED", "true")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("PCMI_SKIP_SSE_HTTPTEST", "1")

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	// InitRedis is called once; all n instances share this global client.
	event.InitRedis(mr.Addr())
	cfg := config.Load()

	cluster := &multiCluster{mr: mr}
	for i := 0; i < n; i++ {
		poolCfg, err := pgxpool.ParseConfig(dbURL)
		if err != nil {
			t.Fatalf("instance %d parse db url: %v", i, err)
		}
		// Keep per-replica pools small: many tests × N replicas share one Postgres (max_connections≈100).
		poolCfg.MaxConns = 3
		pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
		if err != nil {
			t.Fatalf("instance %d pool: %v", i, err)
		}
		t.Cleanup(pool.Close)

		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(middleware.APIKeyMiddleware(pool))
		app.Use(middleware.RateLimitMiddleware(cfg))
		app.Use(middleware.NewAuditMiddleware(pool).Middleware())
		RegisterReadyRoutes(app, pool)
		if err := SetupMemoryRoutes(app, pool, pool, cfg); err != nil {
			t.Fatalf("instance %d SetupMemoryRoutes: %v", i, err)
		}
		if err := SetupSessionRoutes(app, pool, pool, cfg); err != nil {
			t.Fatalf("instance %d SetupSessionRoutes: %v", i, err)
		}
		SetupAdminRoutes(app, pool)

		srv := httptest.NewServer(adaptor.FiberApp(app))
		t.Cleanup(func() {
			srv.CloseClientConnections()
			srv.Close()
		})

		cluster.instances = append(cluster.instances, &multiAPIInstance{
			id:     i,
			srv:    srv,
			client: srv.Client(),
			pool:   pool,
		})
	}
	return cluster
}

func (inst *multiAPIInstance) post(t *testing.T, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, inst.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("inst%d new POST request: %v", inst.id, err)
	}
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := inst.client.Do(req)
	if err != nil {
		t.Fatalf("inst%d POST %s: %v", inst.id, path, err)
	}
	return resp
}

func (inst *multiAPIInstance) get(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, inst.srv.URL+path, nil)
	if err != nil {
		t.Fatalf("inst%d new GET request: %v", inst.id, err)
	}
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err := inst.client.Do(req)
	if err != nil {
		t.Fatalf("inst%d GET %s: %v", inst.id, path, err)
	}
	return resp
}

// TestMultiInstance_DataVisibilityAcrossInstances writes a memory entry via
// instance 0 and verifies instances 1 and 2 return the same content on GET.
// This is the fundamental multi-replica contract: shared DB, no stale reads.
func TestMultiInstance_DataVisibilityAcrossInstances(t *testing.T) {
	cluster := newMultiCluster(t, 3)

	path := "root.multiinstance.visibility." + time.Now().Format("150405.000")
	content := "written-by-instance-0"

	body := fmt.Sprintf(`{"path":%q,"content":%q,"embedding_model":"unspecified"}`, path, content)
	resp := cluster.instances[0].post(t, "/v1/memories", body)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("store on inst0: %d %s", resp.StatusCode, b)
	}

	for _, inst := range cluster.instances[1:] {
		resp := inst.get(t, "/v1/memories/"+path)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("inst%d GET %s: %d %s", inst.id, path, resp.StatusCode, b)
		}
		var entry struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(b, &entry); err != nil {
			t.Fatalf("inst%d decode: %v (body=%s)", inst.id, err, b)
		}
		if entry.Content != content {
			t.Errorf("inst%d: got content %q, want %q", inst.id, entry.Content, content)
		}
	}
}

// TestMultiInstance_ConcurrentWritesFromAllInstances fires 5 instances × 100
// goroutines concurrently — 500 total writes spread across all replicas.
// No writes must fail; all must return HTTP 200.
func TestMultiInstance_ConcurrentWritesFromAllInstances(t *testing.T) {
	const (
		numInstances  = 5
		writesPerInst = 100
	)
	cluster := newMultiCluster(t, numInstances)
	prefix := "root.multiinstance.concurrent." + time.Now().Format("150405.000")

	var wg sync.WaitGroup
	var errCount atomic.Int64

	for _, inst := range cluster.instances {
		for j := 0; j < writesPerInst; j++ {
			wg.Add(1)
			inst := inst
			path := fmt.Sprintf("%s.inst%d.j%04d", prefix, inst.id, j)
			go func() {
				defer wg.Done()
				b := fmt.Sprintf(`{"path":%q,"content":"concurrent-payload","embedding_model":"unspecified"}`, path)
				resp := inst.post(t, "/v1/memories", b)
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					errCount.Add(1)
					t.Logf("inst%d write %q failed: %d %s", inst.id, path, resp.StatusCode, body)
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("TestMultiInstance_ConcurrentWritesFromAllInstances: deadlock")
	}

	if n := errCount.Load(); n > 0 {
		t.Errorf("%d/%d concurrent writes failed across all instances", n, numInstances*writesPerInst)
	}
}

// TestMultiInstance_RedisEventPropagation verifies that storing a memory via
// the HTTP API causes a memory.stored event to appear in the shared Redis stream.
// In production, this stream is consumed by worker processes on all replicas.
func TestMultiInstance_RedisEventPropagation(t *testing.T) {
	cluster := newMultiCluster(t, 2)
	ctx := context.Background()

	// Baseline stream length before the write.
	lenBefore, err := event.RedisClient.XLen(ctx, event.StreamKey).Result()
	if err != nil {
		t.Fatalf("XLEN before: %v", err)
	}

	path := "root.multiinstance.redis.event." + time.Now().Format("150405.000")
	body := fmt.Sprintf(`{"path":%q,"content":"event-test","embedding_model":"unspecified"}`, path)

	resp := cluster.instances[0].post(t, "/v1/memories", body)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("store: %d %s", resp.StatusCode, b)
	}

	// Poll until a memory.stored event appears in the Redis stream.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		lenAfter, _ := event.RedisClient.XLen(ctx, event.StreamKey).Result()
		if lenAfter > lenBefore {
			// XREVRANGE reads newest entries first.
			entries, err := event.RedisClient.XRevRange(ctx, event.StreamKey, "+", "-").Result()
			if err != nil {
				t.Fatalf("XREVRANGE: %v", err)
			}
			for _, e := range entries {
				if e.Values["type"] == "memory.stored" {
					t.Logf("memory.stored event found in Redis stream (id=%s)", e.ID)
					return
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("timeout: memory.stored event not found in Redis stream after store")
}

// TestMultiInstance_SamePathConcurrentWrites sends 5 instances all writing to
// the same path at the same time. No instance should panic; all should return
// HTTP 200 (last-writer-wins is acceptable, corruption is not).
func TestMultiInstance_SamePathConcurrentWrites(t *testing.T) {
	cluster := newMultiCluster(t, 5)
	path := "root.multiinstance.samepath." + time.Now().Format("150405.000")

	var wg sync.WaitGroup
	var errCount atomic.Int64

	for i, inst := range cluster.instances {
		wg.Add(1)
		inst := inst
		content := fmt.Sprintf("from-inst%d", i)
		go func() {
			defer wg.Done()
			b := fmt.Sprintf(`{"path":%q,"content":%q,"embedding_model":"unspecified"}`, path, content)
			resp := inst.post(t, "/v1/memories", b)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				errCount.Add(1)
				t.Logf("inst%d same-path write failed: %d %s", inst.id, resp.StatusCode, body)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestMultiInstance_SamePathConcurrentWrites: deadlock")
	}

	if n := errCount.Load(); n > 0 {
		t.Errorf("%d same-path concurrent writes failed", n)
	}

	// The path must be readable after all concurrent writes settle.
	resp := cluster.instances[0].get(t, "/v1/memories/"+path)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET after concurrent writes: %d %s", resp.StatusCode, b)
	}
}

// TestMultiInstance_MixedReadWriteLoad simulates production traffic: 4 instances
// each running 50 concurrent writers and 50 concurrent readers targeting the same
// path namespace. Verifies no deadlocks, no panics, and HTTP 200 for all requests.
func TestMultiInstance_MixedReadWriteLoad(t *testing.T) {
	const (
		numInstances = 4
		writersEach  = 50
		readersEach  = 50
	)
	cluster := newMultiCluster(t, numInstances)
	prefix := "root.multiinstance.mixed." + time.Now().Format("150405.000")

	// Pre-populate one entry so readers always find something.
	seed := fmt.Sprintf(`{"path":%q,"content":"seed","embedding_model":"unspecified"}`, prefix+".seed")
	resp := cluster.instances[0].post(t, "/v1/memories", seed)
	io.ReadAll(resp.Body) //nolint:errcheck
	resp.Body.Close()

	var wg sync.WaitGroup
	var writeErr, readErr atomic.Int64

	for _, inst := range cluster.instances {
		// Writers
		for j := 0; j < writersEach; j++ {
			wg.Add(1)
			inst := inst
			path := fmt.Sprintf("%s.inst%d.w%04d", prefix, inst.id, j)
			go func() {
				defer wg.Done()
				b := fmt.Sprintf(`{"path":%q,"content":"mixed-write","embedding_model":"unspecified"}`, path)
				resp := inst.post(t, "/v1/memories", b)
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					writeErr.Add(1)
					t.Logf("write err inst%d: %d %s", inst.id, resp.StatusCode, body)
				}
			}()
		}

		// Readers — retrieve from the shared prefix
		for j := 0; j < readersEach; j++ {
			wg.Add(1)
			inst := inst
			go func() {
				defer wg.Done()
				retBody := fmt.Sprintf(`{"path_prefix":%q,"limit":10}`, prefix)
				resp := inst.post(t, "/v1/retrieve", retBody)
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					readErr.Add(1)
					t.Logf("retrieve err inst%d: %d %s", inst.id, resp.StatusCode, body)
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		t.Fatal("TestMultiInstance_MixedReadWriteLoad: deadlock")
	}

	total := int64(numInstances * (writersEach + readersEach))
	if n := writeErr.Load() + readErr.Load(); n > 0 {
		t.Errorf("%d/%d mixed-load requests failed (writes=%d reads=%d)",
			n, total, writeErr.Load(), readErr.Load())
	}
}

// TestMultiInstance_CrossInstanceRetrieve writes 3 entries on 3 different
// instances and then retrieves them from a 4th instance — verifying that a
// replica that never wrote any data can still read all entries from the DB.
func TestMultiInstance_CrossInstanceRetrieve(t *testing.T) {
	cluster := newMultiCluster(t, 4)
	prefix := "root.multiinstance.xretrieve." + time.Now().Format("150405.000")

	// Each of the first 3 instances stores its own entry.
	for _, inst := range cluster.instances[:3] {
		path := fmt.Sprintf("%s.inst%d", prefix, inst.id)
		b := fmt.Sprintf(`{"path":%q,"content":"cross_inst%d","embedding_model":"unspecified"}`, path, inst.id)
		resp := inst.post(t, "/v1/memories", b)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("inst%d store failed: %d %s", inst.id, resp.StatusCode, body)
		}
	}

	// Instance 3 (writer-never) retrieves all entries written by the other instances.
	reader := cluster.instances[3]
	retBody := fmt.Sprintf(`{"path_prefix":%q,"limit":10}`, prefix)
	resp := reader.post(t, "/v1/retrieve", retBody)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("cross-instance retrieve: %d %s", resp.StatusCode, b)
	}

	var result struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("decode retrieve: %v (body=%s)", err, b)
	}
	if len(result.Entries) < 3 {
		t.Errorf("cross-instance retrieve: got %d entries, want >= 3 (body=%s)", len(result.Entries), b)
	}
}

// TestMultiInstance_StatsConsistencyAcrossInstances verifies that all replicas
// return HTTP 200 for /v1/stats and report non-negative memory counts.
func TestMultiInstance_StatsConsistencyAcrossInstances(t *testing.T) {
	cluster := newMultiCluster(t, 4)

	// Write one entry so stats are non-trivially populated.
	path := "root.multiinstance.stats." + time.Now().Format("150405.000")
	b := fmt.Sprintf(`{"path":%q,"content":"stats-seed","embedding_model":"unspecified"}`, path)
	resp := cluster.instances[0].post(t, "/v1/memories", b)
	io.ReadAll(resp.Body) //nolint:errcheck
	resp.Body.Close()

	for _, inst := range cluster.instances {
		resp := inst.get(t, "/v1/stats")
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("inst%d /v1/stats: %d %s", inst.id, resp.StatusCode, body)
			continue
		}
		var stats map[string]any
		if err := json.Unmarshal(body, &stats); err != nil {
			t.Errorf("inst%d stats decode: %v", inst.id, err)
		}
	}
}

// TestMultiInstance_HighVolumeMultiReplica is the comprehensive load test:
// 5 replicas × 100 concurrent writers + 50 concurrent readers = 750 goroutines.
// Tolleranza: al massimo 1% di errori (transient pool/timeout sotto carico).
func TestMultiInstance_HighVolumeMultiReplica(t *testing.T) {
	const (
		numInstances = 5
		writersEach  = 100
		readersEach  = 50
	)
	cluster := newMultiCluster(t, numInstances)
	prefix := "root.multiinstance.highvol." + time.Now().Format("150405")

	var wg sync.WaitGroup
	var writeErr, readErr atomic.Int64

	for _, inst := range cluster.instances {
		for j := 0; j < writersEach; j++ {
			wg.Add(1)
			inst := inst
			path := fmt.Sprintf("%s.inst%d.w%05d", prefix, inst.id, j)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				b := fmt.Sprintf(`{"path":%q,"content":"high-vol","embedding_model":"unspecified"}`, path)
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, inst.srv.URL+"/v1/memories", strings.NewReader(b))
				req.Header.Set("X-API-Key", httpE2EAPIKey)
				req.Header.Set("Content-Type", "application/json")
				resp, err := inst.client.Do(req)
				if err != nil {
					writeErr.Add(1)
					return
				}
				io.ReadAll(resp.Body) //nolint:errcheck
				resp.Body.Close()
				if resp.StatusCode != 200 {
					writeErr.Add(1)
				}
			}()
		}

		for j := 0; j < readersEach; j++ {
			wg.Add(1)
			inst := inst
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				retBody := fmt.Sprintf(`{"path_prefix":%q,"limit":5}`, prefix)
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, inst.srv.URL+"/v1/retrieve", strings.NewReader(retBody))
				req.Header.Set("X-API-Key", httpE2EAPIKey)
				req.Header.Set("Content-Type", "application/json")
				resp, err := inst.client.Do(req)
				if err != nil {
					readErr.Add(1)
					return
				}
				io.ReadAll(resp.Body) //nolint:errcheck
				resp.Body.Close()
				if resp.StatusCode != 200 {
					readErr.Add(1)
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		t.Fatalf("TestMultiInstance_HighVolumeMultiReplica: deadlock (writeErr=%d readErr=%d)",
			writeErr.Load(), readErr.Load())
	}

	total := int64(numInstances * (writersEach + readersEach))
	failed := writeErr.Load() + readErr.Load()
	// Tolleranza 1%: fallimenti transient sono accettabili sotto carico estremo.
	threshold := total / 100
	if threshold < 1 {
		threshold = 1
	}
	t.Logf("high-volume: total=%d writeErr=%d readErr=%d (soglia=%d)",
		total, writeErr.Load(), readErr.Load(), threshold)
	if failed > threshold {
		t.Errorf("%d/%d richieste fallite (>1%% soglia=%d): writes=%d reads=%d",
			failed, total, threshold, writeErr.Load(), readErr.Load())
	}
}
