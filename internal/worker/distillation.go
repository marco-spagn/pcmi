package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/log"
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
		log.Warn("OPENAI_API_KEY unset, distillation LLM calls will fail")
	}
	log.Info("distillation worker initialized", "concurrency", concurrency)
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
	log.Info("distillation engine started", "batch_size", w.batchSize)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	defaultTenant := "00000000-0000-0000-0000-000000000000"

	for {
		select {
		case <-ctx.Done():
			log.Info("distillation worker stopped")
			return
		case <-ticker.C:
			log.Debug("distillation fallback timer tick")
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
		log.Error("distillation set tenant failed", "err", err)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}

	batchSize := w.batchSize
	log.Info("distillation job started", "tenant", tenantID, "path_prefix", pathPrefix, "batch_size", batchSize)

	rows, err := w.db.Query(ctx, distillationSourceEntriesSQL(), tenantID, pathPrefix, batchSize)
	if err != nil {
		log.Error("distillation query failed", "err", err)
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
		log.Info("distillation skipped, no source memories", "path_prefix", pathPrefix)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "skipped")
		return
	}

	metrics.ObserveDistillationSources(len(entries))

	if w.apiKey == "" {
		log.Warn("skipping LLM distillation, no OPENAI_API_KEY set")
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "skipped")
		return
	}

	log.Info("distilling memories", "count", len(entries), "path_prefix", pathPrefix)

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
		log.Error("LLM distillation error", "err", err)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}

	// BUG-FIX-1: guard against empty Choices (OpenAI API can return 0 choices on
	// content-filter or rate-limit soft errors without returning an HTTP error code).
	if len(resp.Choices) == 0 {
		log.Error("LLM returned 0 choices (finish_reason may be content_filter)")
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "error")
		return
	}
	result, err := parseDistillationLLMResponse(resp.Choices[0].Message.Content)
	if err != nil {
		log.Error("LLM response JSON parse failed", "err", err, "raw", resp.Choices[0].Message.Content)
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
		log.Error("distillation dedup check failed", "err", err)
		return
	}
	if dup {
		log.Info("distillation skipped, duplicate sources", "path", distilledPath, "tenant", tenantID)
		metrics.ObserveDistillationJob(time.Since(start).Seconds(), "duplicate")
		return
	}

	version, err := nextDistilledVersion(ctx, w.db, tenantID, distilledPath)
	if err != nil {
		log.Error("distillation version lookup failed", "err", err)
		return
	}

	insightsJSON, err := json.Marshal(result.Insights)
	if err != nil {
		log.Error("distillation insights marshal failed", "err", err)
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
		log.Error("insert distilled knowledge failed", "err", err)
		return
	}

	if err := event.PublishEvent(event.EventKnowledgeDistilled, map[string]any{
		"id":        distilledID,
		"tenant_id": tenantID,
		"path":      distilledPath,
		"version":   version,
		"sources":   len(sourceIDs),
	}); err != nil {
		log.Warn("distilled event publish failed", "err", err)
	}

	metrics.ObserveDistillationJob(time.Since(start).Seconds(), "ok")
	log.Info("distillation saved", "id", distilledID, "path", distilledPath, "version", version, "tenant", tenantID, "sources", len(sourceIDs))

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
	if w.db == nil {
		return
	}
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
		log.Warn("markRunCompleted failed", "err", err)
	}
}
