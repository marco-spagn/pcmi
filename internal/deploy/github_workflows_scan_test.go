package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAllGitHubWorkflowYAMLFilesParse walks .github/workflows/*.yml so a stray
// syntax error (or duplicate key at parse time) fails tests before push.
func TestAllGitHubWorkflowYAMLFilesParse(t *testing.T) {
	repo := repoRoot(t)
	dir := filepath.Join(repo, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read workflows dir: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		found = true
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var doc interface{}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("YAML parse %s: %v", path, err)
		}
	}
	if !found {
		t.Fatal("no .yml workflows found under .github/workflows")
	}
}
