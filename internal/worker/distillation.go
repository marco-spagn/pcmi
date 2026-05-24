package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/sashabaranov/go-openai"
)

type DistillationWorker struct {
	db         *pgxpool.Pool
	openai     *openai.Client
	modelName  string
	apiKey     string
	batchSize  int
	sem        chan struct{}
}

func NewDistillationWorker(db *pgxpool.Pool, cfg *config.Config) *DistillationWorker {
	apiKey := ""
	model := "gpt-4o-mini"
	batchSize := defaultDistillationBatchSize
	concurrency := defaultDistillationConcurrency
	if cfg != nil {
		apiKey = cfg.OpenAIAPIKey
		if strings.TrimSpace(cfg.DistillationModel) != "" {
			model = strings.TrimSpace(cfg.DistillationModel)
		}
		batchSize = distillationBatchSizeFrom(cfg.DistillationBatchSize)
		concurrency = distillationConcurrencyFrom(cfg.DistillationConcurrency)
	}
	if apiKey == "" {
		log.Println("⚠️  OPENAI_API_KEY unset — distillation LLM calls will fail")
	}
	log.Printf("🔧 Distillation concurrency limit: %d parallel LLM jobs", concurrency)
	return &DistillationWorker{
		db:        db,
		openai:    openai.NewClient(apiKey),
		modelName: model,
		apiKey:    apiKey,
		batchSize: batchSize,
		sem:       make(chan struct{}, concurrency),
	}
}

func (w *DistillationWorker) TriggerForMemory(tenantID, path string) {
	metrics.IncDistillationQueued()
	go func() {
		w.sem <- struct{}{}
		metrics.DecDistillationQueued()
		metrics.IncDistillationActive()
		defer func() {
			<-w.sem
			metrics.DecDistillationActive()
		}()
		w.runDistillationJob(tenantID, path)
	}()
}

func (w *DistillationWorker) TriggerForPrefix(tenantID, pathPrefix string) {
	metrics.IncDistillationQueued()
	go func() {
		w.sem <- struct{}{}
		metrics.DecDistillationQueued()
		metrics.IncDistillationActive()
		defer func() {
			<-w.sem
			metrics.DecDistillationActive()
		}()
		w.runDistillationJobExact(tenantID, pathPrefix)
	}()
}

func (w *DistillationWorker) runDistillationJobExact(tenantID, pathPrefix string) {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}
	pathPrefix = strings.TrimSpace(pathPrefix)
	if pathPrefix == "" {
		pathPrefix = "root"
	}
	w.runDistillationJobWithPrefix(tenantID, pathPrefix)
}

func (w *DistillationWorker) runDistillationJob(tenantID, memoryPath string) {
	pathPrefix := DistillPathPrefix(memoryPath)
	w.runDistillationJobWithPrefix(tenantID, pathPrefix)
}

func (w *DistillationWorker) Start(ctx context.Context) {
	log.Printf("🚀 Distillation Engine v1.7 started — Redis-driven + fallback timer (batch_size=%d)", w.batchSize)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	defaultTenant := "00000000-0000-0000-0000-000000000000"

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Distillation worker stopped")
			return
		case <-ticker.C:
			log.Println("⏰ Fallback timer: periodic distillation for default tenant")
			w.runDistillationJob(defaultTenant, "root.test")
		}
	}
}

func (w *DistillationWorker) runDistillationJobWithPrefix(tenantID, pathPrefix string) {
	start := time.Now()
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}
	distilledPath := distilledBranchPath(pathPrefix)

	// FIX-3: use a bounded context so a slow DB or OpenAI call never blocks
	// a semaphore slot indefinitely. 3 min is generous for batch=10 with LLM.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if _, err := w.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		log.Printf("❌ distillation set tenant: %v", err)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}

	batchSize := w.batchSize
	log.Printf("🔄 Distillation job tenant=%s path_prefix=%s batch_size=%d", tenantID, pathPrefix, batchSize)

	rows, err := w.db.Query(ctx, distillationSourceEntriesSQL(), tenantID, pathPrefix, batchSize)
	if err != nil {
		log.Printf("❌ distillation query error: %v", err)
		return
	}
	defer rows.Close()

	var entries []struct {
		ID       int64
		Content  string
		Metadata map[string]any
	}
	for rows.Next() {
		var e struct {
			ID       int64
			Content  string
			Metadata []byte
		}
		if err := rows.Scan(&e.ID, &e.Content, &e.Metadata); err != nil {
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal(e.Metadata, &meta)
		entries = append(entries, struct {
			ID       int64
			Content  string
			Metadata map[string]any
		}{e.ID, e.Content, meta})
	}

	if len(entries) < 1 {
		log.Printf("📊 No memories under %s — distillation skipped", pathPrefix)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "skipped")
		return
	}

	metrics.ObserveDistillationSources(len(entries))

	if w.apiKey == "" {
		log.Println("⚠️  Skipping LLM distillation (no OPENAI_API_KEY)")
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "skipped")
		return
	}

	log.Printf("🧠 Distilling %d raw memories under %s...", len(entries), pathPrefix)

	prompt := `Summarize these memories into higher-order knowledge.
Return ONLY valid JSON:
{"summary": "...", "insights": ["insight1", "insight2"]}`

	var messages []openai.ChatCompletionMessage
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: prompt,
	})
	for _, e := range entries {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: e.Content,
		})
	}

	resp, err := w.openai.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    w.modelName,
		Messages: messages,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		log.Printf("❌ LLM distillation error: %v", err)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}

	// BUG-FIX-1: guard against empty Choices (OpenAI API can return 0 choices on
	// content-filter or rate-limit soft errors without returning an HTTP error code).
	if len(resp.Choices) == 0 {
		log.Printf("❌ LLM returned 0 choices (finish_reason may be 'content_filter')")
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}
	result, err := parseDistillationLLMResponse(resp.Choices[0].Message.Content)
	if err != nil {
		log.Printf("❌ JSON parse error: %v (raw: %s)", err, resp.Choices[0].Message.Content)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}

	sourceIDs := make([]int64, len(entries))
	for i, e := range entries {
		sourceIDs[i] = e.ID
	}
	sourceIDs = normalizeSourceIDs(sourceIDs)

	dup, err := w.hasDuplicateDistillation(ctx, tenantID, distilledPath, sourceIDs)
	if err != nil {
		log.Printf("❌ distillation dedup check: %v", err)
		return
	}
	if dup {
		log.Printf("⏭️  Distillation skipped (duplicate sources) path=%s tenant=%s", distilledPath, tenantID)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "duplicate")
		return
	}

	version, err := nextDistilledVersion(ctx, w.db, tenantID, distilledPath)
	if err != nil {
		log.Printf("❌ distillation version: %v", err)
		return
	}

	insightsJSON, err := json.Marshal(result.Insights)
	if err != nil {
		log.Printf("❌ insights marshal: %v", err)
		return
	}

	var distilledID int64
	err = w.db.QueryRow(ctx, `
		INSERT INTO distilled_knowledge (
			tenant_id, path, summary, insights, confidence_score, source_entry_ids, version
		) VALUES ($1::uuid, $2::ltree, $3, $4::jsonb, $5, $6, $7)
		RETURNING id`,
		tenantID,
		distilledPath,
		result.Summary,
		insightsJSON,
		0.85,
		sourceIDs,
		version,
	).Scan(&distilledID)
	if err != nil {
		log.Printf("❌ insert distilled error: %v", err)
		return
	}

	if err := event.PublishEvent(event.EventKnowledgeDistilled, map[string]any{
		"id":        distilledID,
		"tenant_id": tenantID,
		"path":      distilledPath,
		"version":   version,
		"sources":   len(sourceIDs),
	}); err != nil {
		log.Printf("⚠️  distilled event publish: %v", err)
	}

	metrics.ObserveDistillationJob(time.Since(start).Seconds(), "ok")
	log.Printf("✅ Distillation saved id=%d at %s v%d (tenant=%s, sources=%d)",
		distilledID, distilledPath, version, tenantID, len(sourceIDs))

	// FIX-4: update distillation_runs row to 'completed' when called from
	// the policy engine (runID > 0). Standalone Redis-triggered calls pass
	// runID = 0 and this is a no-op.
	w.markRunCompleted(ctx, tenantID, distilledID)
}

func (w *DistillationWorker) hasDuplicateDistillation(ctx context.Context, tenantID, distilledPath string, sourceIDs []int64) (bool, error) {
	var existingID int64
	err := w.db.QueryRow(ctx, `
		SELECT id FROM distilled_knowledge
		WHERE tenant_id = $1::uuid
		  AND path = $2::ltree
		  AND source_entry_ids = $3::bigint[]
		LIMIT 1`, tenantID, distilledPath, sourceIDs).Scan(&existingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// markRunCompleted updates the most recent distillation_runs row for this
// tenant+path from 'running' to 'completed' and records the distilled_id.
// FIX-4: without this, all distillation_runs rows stayed in 'running' forever
// regardless of outcome, making the runs history table useless for monitoring.
func (w *DistillationWorker) markRunCompleted(ctx context.Context, tenantID string, distilledID int64) {
	_, err := w.db.Exec(ctx, `
		UPDATE distillation_runs
		SET    status       = 'completed',
		       distilled_id = $3,
		       completed_at = NOW()
		WHERE  tenant_id = $1::uuid
		  AND  status    = 'running'
		  AND  id = (
		       SELECT id FROM distillation_runs
		       WHERE  tenant_id = $1::uuid AND status = 'running'
		       ORDER BY created_at DESC LIMIT 1
		  )`, tenantID, tenantID, distilledID)
	if err != nil {
		// Non-fatal: the distillate was saved; only the audit row is wrong.
		log.Printf("⚠️ markRunCompleted: %v", err)
	}
}
