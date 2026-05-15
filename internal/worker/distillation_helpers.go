package worker

import (
	"slices"
	"strings"
)

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
