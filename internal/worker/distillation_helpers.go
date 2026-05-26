package worker

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/marco-spagn/pcmi/internal/log"
)

// defaultDistillationBatchSize is the max number of raw memories per distillation job.
const defaultDistillationBatchSize = 10

// defaultDistillationConcurrency is the max number of LLM distillation jobs that
// can run in parallel.
const defaultDistillationConcurrency = 4

func distillationBatchSizeFrom(cfgBatch int) int {
	if cfgBatch >= 1 && cfgBatch <= 200 {
		return cfgBatch
	}
	if cfgBatch != 0 {
		log.Warn("distillation batch size invalid, using default", "configured", cfgBatch, "default", defaultDistillationBatchSize)
	}
	return defaultDistillationBatchSize
}

func distillationConcurrencyFrom(cfgConcurrency int) int {
	if cfgConcurrency >= 1 && cfgConcurrency <= 16 {
		return cfgConcurrency
	}
	if cfgConcurrency != 0 {
		log.Warn("distillation concurrency invalid, using default", "configured", cfgConcurrency, "default", defaultDistillationConcurrency)
	}
	return defaultDistillationConcurrency
}

// distilledBranchPath is the ltree path where distilled knowledge is stored for a prefix.
func distilledBranchPath(pathPrefix string) string {
	return pathPrefix + ".distilled"
}

// distillationSourceEntriesSQL selects raw memories eligible for distillation under pathPrefix.
// BUG-FIX-2: rows at or under prefix.distilled are excluded to avoid re-distillation loops.
func distillationSourceEntriesSQL() string {
	return `
		SELECT id, content, metadata
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND path <@ $2::ltree
		  AND NOT (path <@ (($2::text || '.distilled')::ltree))
		  AND valid_to IS NULL
		ORDER BY created_at DESC
		LIMIT $3`
}

// DistillPathPrefix maps a memory path to the ltree prefix used for distillation.
func DistillPathPrefix(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "root.test"
	}
	if strings.HasPrefix(path, "root.test") {
		return "root.test"
	}
	parts := strings.Split(path, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return path
}

func normalizeSourceIDs(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	slices.Sort(out)
	return out
}

func sourceIDsEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	na := normalizeSourceIDs(a)
	nb := normalizeSourceIDs(b)
	return slices.Equal(na, nb)
}

var trailingCommaBeforeClose = regexp.MustCompile(`,(\s*[}\]])`)

type distillationLLMResult struct {
	Summary  string   `json:"summary"`
	Insights []string `json:"insights"`
}

// parseDistillationLLMResponse extracts summary/insights from model output, repairing common JSON defects.
func parseDistillationLLMResponse(raw string) (distillationLLMResult, error) {
	content := sanitizeDistillationJSON(raw)
	var result distillationLLMResult
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return result, nil
	}
	repaired := repairDistillationJSON(content)
	if err := json.Unmarshal([]byte(repaired), &result); err != nil {
		return distillationLLMResult{}, err
	}
	return result, nil
}

func sanitizeDistillationJSON(raw string) string {
	content := strings.TrimSpace(raw)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func repairDistillationJSON(content string) string {
	content = trailingCommaBeforeClose.ReplaceAllString(content, `$1`)
	if idx := strings.Index(content, `"insights"`); idx >= 0 {
		tail := content[idx:]
		missing := strings.Count(tail, "[") - strings.Count(tail, "]")
		if missing > 0 {
			if close := strings.LastIndex(content, "}"); close >= 0 {
				content = content[:close] + strings.Repeat("]", missing) + content[close:]
			}
		}
	}
	return content
}
