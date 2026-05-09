package worker

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
		log.Println("⚠️  OPENAI_API_KEY non trovata – distillation userà modalità simulata")
	}
	return &DistillationWorker{
		db:        db,
		openai:    openai.NewClient(apiKey),
		modelName: "gpt-4o-mini",
	}
}

func (w *DistillationWorker) Start(ctx context.Context) {
	log.Println("🚀 Distillation Engine started – raffinamento automatico conoscenza")

	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Distillation worker stopped")
			return
		case <-ticker.C:
			w.runDistillationJob()
		}
	}
}

func (w *DistillationWorker) runDistillationJob() {
	log.Println("🔄 Avvio job di distillation su subtree root.test...")

	// Recupera ricordi grezzi
	rows, err := w.db.Query(context.Background(), `
		SELECT id, content, metadata 
		FROM memory_entries 
		WHERE path::text LIKE 'root.test%' 
		  AND valid_to IS NULL 
		ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		log.Printf("distillation query error: %v", err)
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
		json.Unmarshal(e.Metadata, &meta)
		entries = append(entries, struct {
			ID       int64
			Content  string
			Metadata map[string]any
		}{e.ID, e.Content, meta})
	}

	if len(entries) < 2 {
		log.Printf("📊 Trovati solo %d ricordi – distillazione saltata", len(entries))
		return
	}

	log.Printf("🧠 Distillando %d ricordi grezzi...", len(entries))

	// Prompt per LLM
	prompt := `Riassumi questi ricordi in un unico insight di ordine superiore.
Genera:
1. Un summary conciso (max 2 righe)
2. Una lista di insights chiave come array JSON
Formato risposta JSON:
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

	resp, err := w.openai.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    w.modelName,
		Messages: messages,
	})
	if err != nil {
		log.Printf("LLM distillation error: %v", err)
		return
	}

	var result struct {
		Summary  string   `json:"summary"`
		Insights []string `json:"insights"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		log.Printf("JSON parse error: %v", err)
		return
	}

	// Salva nella tabella distilled_knowledge
	sourceIDs := make([]int64, len(entries))
	for i, e := range entries {
		sourceIDs[i] = e.ID
	}

	_, err = w.db.Exec(context.Background(), `
		INSERT INTO distilled_knowledge (
			tenant_id, path, summary, insights, confidence_score, source_entry_ids
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		"00000000-0000-0000-0000-000000000000",
		"root.test.distilled",
		result.Summary,
		result.Insights,
		0.85,
		sourceIDs,
	)
	if err != nil {
		log.Printf("insert distilled error: %v", err)
		return
	}

	log.Println("✅ Distillazione completata – conoscenza di ordine superiore salvata in distilled_knowledge")
}
