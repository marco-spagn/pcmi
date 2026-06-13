package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

// ContradictionWorker scans recently stored memories for contradictory claims
// and creates contradict edges in memory_links when detected.
type ContradictionWorker struct {
	db   *pgxpool.Pool
	repo *repository.MemoryRepository
	cfg  *config.Config
	mu   sync.Mutex
}

// NewContradictionWorker creates a new contradiction detection worker.
func NewContradictionWorker(db *pgxpool.Pool, cfg *config.Config) *ContradictionWorker {
	return &ContradictionWorker{
		db:   db,
		repo: repository.NewMemoryRepository(db, nil),
		cfg:  cfg,
	}
}

// Start begins the periodic background scan for contradictions.
func (w *ContradictionWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(w.cfg.ContradictionDetectionIntervalSecs) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scanRecent(ctx)
		}
	}
}

// OnMemoryEvent triggers contradiction detection when a new memory is stored.
func (w *ContradictionWorker) OnMemoryEvent(tenantID, path string) {
	if tenantID == "" || path == "" {
		return
	}
	if w.db == nil {
		return
	}
	prefix := parentPrefix(path)
	if prefix == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = w.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)
		if err := w.checkPrefix(ctx, tenantID, prefix); err != nil {
			log.Printf("⚠️ contradiction detection: %v", err)
		}
	}()
}

// checkPrefix queries memories under prefix (excluding .consolidated) and runs
// pairwise contradiction heuristics. Contradictory pairs are written as
// contradicts links in memory_links.
func (w *ContradictionWorker) checkPrefix(ctx context.Context, tenantID, prefix string) error {
	// NOTE: the source column is `content` (there is no `text_content` column),
	// and consolidated entries are excluded with a plain LIKE on the ltree text —
	// the previous `path !~ '*.consolidated.*'` was neither a valid lquery match
	// nor a valid POSIX regex, so this query errored on every run and the whole
	// feature silently did nothing. Only current rows (valid_to IS NULL) are
	// compared so a memory never contradicts its own superseded versions.
	rows, err := w.db.Query(ctx, `
		SELECT id, path, content
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND ($2::text = '' OR path <@ $2::ltree)
		  AND valid_to IS NULL
		  AND NOT (path::text LIKE '%.consolidated' OR path::text LIKE '%.consolidated.%')
		ORDER BY created_at DESC
		LIMIT 50`, tenantID, prefix)
	if err != nil {
		return fmt.Errorf("query memories under prefix: %w", err)
	}
	defer rows.Close()

	type mem struct {
		id   int64
		path string
		text string
	}
	var memories []mem
	for rows.Next() {
		var m mem
		if err := rows.Scan(&m.id, &m.path, &m.text); err != nil {
			return fmt.Errorf("scan memory: %w", err)
		}
		memories = append(memories, m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	if len(memories) < 2 {
		return nil
	}

	linksRepo := repository.NewLinksRepository(w.db, nil)
	for i := 0; i < len(memories); i++ {
		for j := i + 1; j < len(memories); j++ {
			contradicts, confidence := detectContradiction(memories[i].text, memories[j].text)
			if !contradicts {
				continue
			}
			linkReq := model.CreateLinkRequest{
				FromPath: memories[i].path,
				ToPath:   memories[j].path,
				LinkType: "contradicts",
			}
			if _, err := linksRepo.Create(ctx, tenantID, linkReq); err != nil {
				log.Printf("⚠️ contradiction link create: %v", err)
				continue
			}
			payload := map[string]any{
				"tenant_id":  tenantID,
				"from_path":  memories[i].path,
				"to_path":    memories[j].path,
				"confidence": confidence,
			}
			_ = event.PublishEvent(event.EventContradictionDetected, payload)
			log.Printf("🔍 contradiction detected: %s ↔ %s (confidence=%.2f)", memories[i].path, memories[j].path, confidence)
		}
	}
	return nil
}

// scanRecent is the periodic fallback that checks all tenants for recent contradictions.
func (w *ContradictionWorker) scanRecent(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	rows, err := w.db.Query(ctx, `
		SELECT DISTINCT tenant_id::text
		FROM memory_entries
		WHERE created_at > NOW() - INTERVAL '5 minutes'`)
	if err != nil {
		log.Printf("⚠️ contradiction scanRecent: %v", err)
		return
	}
	defer rows.Close()

	var tenantIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			continue
		}
		tenantIDs = append(tenantIDs, tid)
	}

	for _, tid := range tenantIDs {
		if err := w.checkPrefix(ctx, tid, ""); err != nil {
			log.Printf("⚠️ contradiction scanRecent tenant=%s: %v", tid, err)
		}
	}
}

// detectContradiction runs a heuristic contradiction check between two memory texts.
// Returns true and a confidence score (0–1) if the two memories appear to
// contradict each other. v2.0 uses keyword heuristics; LLM comparison is
// planned for a future version.
func detectContradiction(a, b string) (bool, float64) {
	normA := strings.ToLower(strings.TrimSpace(a))
	normB := strings.ToLower(strings.TrimSpace(b))
	if normA == "" || normB == "" {
		return false, 0
	}

	negations := []string{
		"not ", "no ", "never ", "incorrect", "wrong", "false",
		"doesn't", "does not", "isn't", "is not", "aren't", "are not",
		"wasn't", "was not", "weren't", "were not",
	}

	negCountA, negCountB := 0, 0
	for _, neg := range negations {
		negCountA += strings.Count(normA, neg)
		negCountB += strings.Count(normB, neg)
	}

	if len(normA) < 20 || len(normB) < 20 {
		return false, 0
	}

	if negCountA == 0 && negCountB == 0 {
		return false, 0
	}

	wordsA := tokenize(normA)
	wordsB := tokenize(normB)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false, 0
	}

	overlap := wordOverlap(wordsA, wordsB)
	if overlap < 0.15 {
		return false, 0
	}

	confidence := overlap * 0.7
	if negCountA > 0 {
		confidence += 0.15
	}
	if negCountB > 0 {
		confidence += 0.15
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	if confidence < 0.30 {
		return false, 0
	}

	return true, confidence
}

// tokenize splits text into lowercase word tokens, filtering stopwords.
func tokenize(text string) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	stopwords := map[string]bool{
		"the": true, "is": true, "a": true, "an": true, "of": true,
		"to": true, "in": true, "and": true, "it": true, "that": true,
		"this": true, "was": true, "for": true, "on": true, "with": true,
		"as": true, "be": true, "by": true, "at": true, "or": true,
		"from": true, "are": true, "has": true, "been": true, "but": true,
		"not": true, "no": true, "its": true, "can": true, "will": true,
	}
	var result []string
	for _, w := range words {
		if len(w) > 1 && !stopwords[w] {
			result = append(result, w)
		}
	}
	return result
}

// wordOverlap returns the Jaccard similarity between two token sets.
func wordOverlap(a, b []string) float64 {
	setA := make(map[string]bool, len(a))
	for _, w := range a {
		setA[w] = true
	}
	setB := make(map[string]bool, len(b))
	for _, w := range b {
		setB[w] = true
	}

	intersection := 0
	union := len(setA)
	for w := range setB {
		if setA[w] {
			intersection++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
