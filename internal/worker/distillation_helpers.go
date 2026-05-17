package worker

import (
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
)

// defaultDistillationBatchSize is the max number of raw memories per distillation job.
// Override via env var DISTILLATION_BATCH_SIZE.
const defaultDistillationBatchSize = 10

// distillationBatchSize reads DISTILLATION_BATCH_SIZE from the environment (default 10).
// Valid range: 1–200. Values outside this range fall back to the default.
func distillationBatchSize() int {
	raw := strings.TrimSpace(os.Getenv("DISTILLATION_BATCH_SIZE"))
	if raw == "" {
		return defaultDistillationBatchSize
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 200 {
		log.Printf("⚠️  DISTILLATION_BATCH_SIZE=%q invalid, using default %d", raw, defaultDistillationBatchSize)
		return defaultDistillationBatchSize
	}
	return n
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
