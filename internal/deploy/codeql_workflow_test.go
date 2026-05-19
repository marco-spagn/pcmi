package deploy_test

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCodeQLWorkflowYAMLValid guards against invalid YAML / structural drift in
// the CodeQL workflow (VS Code YAML schema errors, truncated files, etc.).
func TestCodeQLWorkflowYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/workflows/codeql.yml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse codeql.yml: %v", err)
	}
	name, _ := root["name"].(string)
	if name != "CodeQL" {
		t.Errorf(`workflow "name": got %q, want "CodeQL"`, name)
	}
	jobs, ok := root["jobs"].(map[string]interface{})
	if !ok || jobs == nil {
		t.Fatal("missing or invalid jobs map")
	}
	if _, ok := jobs["analyze"]; !ok {
		t.Fatal(`missing jobs.analyze`)
	}
	perms, ok := root["permissions"].(map[string]interface{})
	if !ok || perms == nil {
		t.Fatal("missing permissions (required for SARIF upload)")
	}
	if _, ok := perms["security-events"]; !ok {
		t.Fatal(`permissions missing security-events`)
	}
}

// TestCodeQLPackConfigYAMLValid ensures the CodeQL pack config used by init stays parseable.
func TestCodeQLPackConfigYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/codeql/codeql-config.yml"))
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse codeql-config.yml: %v", err)
	}
	paths, ok := root["paths"]
	if !ok || paths == nil {
		t.Fatal("codeql-config.yml missing paths (scan envelope would be empty)")
	}
}
