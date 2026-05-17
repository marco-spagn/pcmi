package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/sashabaranov/go-openai"
)


type DistillationWorker struct {
	db        *pgxpool.Pool
	openai    *openai.Client
	modelName string
}

func NewDistillationWorker(db *pgxpool.Pool) *DistillationWorker {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Println("⚠️  OPENAI_API_KEY unset — distillation LLM calls will fail")
	}
	model := os.Getenv("DISTILLATION_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &DistillationWorker{
		db:        db,
		openai:    openai.NewClient(apiKey),
		modelName: model,
	}
}

func (w *DistillationWorker) TriggerForMemory(tenantID, path string) {
	go w.runDistillationJob(tenantID, path)
}

// TriggerForPrefix runs distillation using path_prefix exactly (refine API).
func (w *DistillationWorker) TriggerForPrefix(tenantID, pathPrefix string) {
	go w.runDistillationJobExact(tenantID, pathPrefix)
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
	log.Printf("🚀 Distillation Engine v1.7 started — Redis-driven + fallback timer (batch_size=%d)", distillationBatchSize())

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
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}
	distilledPath := pathPrefix + ".distilled"

	ctx := context.Background()
	if _, err := w.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		log.Printf("❌ distillation set tenant: %v", err)
		return
	}

	batchSize := distillationBatchSize()
	log.Printf("🔄 Distillation job tenant=%s path_prefix=%s batch_size=%d", tenantID, pathPrefix, batchSize)

	rows, err := w.db.Query(ctx, `
		SELECT id, content, metadata
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND path <@ $2::ltree
		  AND valid_to IS NULL
		ORDER BY created_at DESC
		LIMIT $3`, tenantID, pathPrefix, batchSize)
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
		return
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Println("⚠️  Skipping LLM distillation (no OPENAI_API_KEY)")
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
	})
	if err != nil {
		log.Printf("❌ LLM distillation error: %v", err)
		return
	}

	var result struct {
		Summary  string   `json:"summary"`
		Insights []string `json:"insights"`
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		log.Printf("❌ JSON parse error: %v (raw: %s)", err, resp.Choices[0].Message.Content)
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

	log.Printf("✅ Distillation saved id=%d at %s v%d (tenant=%s, sources=%d)", distilledID, distilledPath, version, tenantID, len(sourceIDs))
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
