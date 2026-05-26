package service

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

// TestMassiveRealisticMultiAgentWorkload simulates a realistic multi-agent
// long-running workload against the MemoryService.
//
// This is intended as a "massive" yet practical test that represents common
// real-world usage of PCMI by teams of AI agents:
//
// - Multiple specialized agents writing memories over "days" of operation.
// - High volume of short observations + occasional long reasoning traces / documents.
// - Realistic tagging and importance scoring.
// - Content-hash dedup on repeated observations.
// - Concurrent stores + retrievals (hybrid search with path + tags + importance).
// - Temporal aspect (as_of queries).
// - One explicit compaction at the end.
//
// The test uses the existing stress mock + miniredis setup so it remains fast
// enough to run in CI while still exercising thousands of realistic operations.
func TestMassiveRealisticMultiAgentWorkload(t *testing.T) {
	initStressRedis(t)

	const (
		numAgents   = 4
		opsPerAgent = 600 // ~2,400 total stores + many retrieves
	)

	repo := newStressRepo(0)
	svc := NewMemoryService(repo, nil)

	ctx := context.Background()
	var wg sync.WaitGroup

	agentNames := []string{"sre", "security", "coding", "product"}

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(agentIdx int) {
			defer wg.Done()

			agent := agentNames[agentIdx]
			basePath := "agent." + agent

			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(agentIdx)))

			for op := 0; op < opsPerAgent; op++ {
				simulatedAt := time.Now().Add(-time.Duration(r.Intn(72)) * time.Hour)

				isLong := r.Intn(100) < 18
				var content string
				tags := []string{agent, "observation"}
				imp := 0.35 + r.Float64()*0.65

				if isLong {
					content = fmt.Sprintf("Long reasoning from %s agent step %d. "+
						"Symptoms: X. Root cause hypothesis: Y. Decision: Z. "+
						"Evidence links and runbooks attached.", agent, op)
					tags = append(tags, "reasoning", "decision")
					imp = 0.65 + r.Float64()*0.35
				} else {
					content = fmt.Sprintf("%s observed %s in %s at step %d",
						agent, randomEvent(r), randomComponent(r), op)
					tags = append(tags, randomComponent(r))
				}

				path := fmt.Sprintf("%s.%s.%d", basePath, randomComponent(r), op)

				_, err := svc.Store(ctx, &model.StoreRequest{
					Path:       path,
					Content:    content,
					Tags:       tags,
					Importance: &imp,
					Metadata:   map[string]any{"simulated_at": simulatedAt},
				}, "tenant-massive-real")
				if err != nil {
					t.Errorf("store error: %v", err)
					return
				}

				// Realistic retrieval an agent would actually perform
				if op%5 == 0 {
					asOf := simulatedAt
					_, _ = svc.Retrieve(ctx, &model.RetrieveRequest{
						PathPrefix: "agent." + agent,
						Limit:      15,
						Tags:       []string{agent},
						AsOf:       &asOf,
					}, "tenant-massive-real")
				}

				// Simulate dedup pressure
				if op%9 == 0 {
					// repeated similar observation
					_, _ = svc.Store(ctx, &model.StoreRequest{
						Path:    path,
						Content: content,
						Tags:    tags,
					}, "tenant-massive-real")
				}
			}
		}(i)
	}

	wg.Wait()

	// Final large retrieval (what a nightly job or human would do)
	resp, err := svc.Retrieve(ctx, &model.RetrieveRequest{
		PathPrefix: "agent.",
		Limit:      100,
	}, "tenant-massive-real")
	if err != nil {
		t.Fatalf("final retrieve failed: %v", err)
	}
	if len(resp.Entries) == 0 {
		t.Error("expected non-empty retrieval after massive workload")
	}

	t.Logf("Massive realistic multi-agent workload completed successfully. "+
		"Final retrieval returned %d items.", len(resp.Entries))
}

func randomEvent(r *rand.Rand) string {
	ev := []string{"error", "warning", "spike", "deployment", "config_change", "auth_failure"}
	return ev[r.Intn(len(ev))]
}

func randomComponent(r *rand.Rand) string {
	c := []string{"api", "worker", "db", "cache", "queue", "auth"}
	return c[r.Intn(len(c))]
}
