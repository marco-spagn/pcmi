package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComposeReferencesExistingMigrations asserts every ./migrations/*.sql bind
// mount declared in docker-compose.yml points at a real file (catches renames).
func TestComposeReferencesExistingMigrations(t *testing.T) {
	repo := repoRoot(t)
	composePath := filepath.Join(repo, "docker-compose.yml")
	raw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	const prefix = "./migrations/"
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, prefix) || !strings.Contains(line, ".sql") {
			continue
		}
		// Typical: - ./migrations/001_init.sql:/docker-entrypoint-initdb.d/...
		part := strings.TrimSpace(strings.Split(line, ":")[0])
		part = strings.TrimPrefix(part, "- ")
		part = strings.Trim(part, `"'`)
		if !strings.HasPrefix(part, prefix) {
			continue
		}
		full := filepath.Join(repo, filepath.FromSlash(strings.TrimPrefix(part, "./")))
		st, err := os.Stat(full)
		if err != nil || st.IsDir() {
			t.Errorf("compose references missing migration file %q", part)
		}
	}
}
