package deploy_test

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCIWorkflowYAMLValid guards the main GitHub Actions workflow: broken YAML
// or dropped jobs surface before merge.
func TestCIWorkflowYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/workflows/ci.yml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	name, _ := root["name"].(string)
	if name != "CI" {
		t.Errorf(`workflow name: got %q, want "CI"`, name)
	}
	jobs, ok := root["jobs"].(map[string]interface{})
	if !ok || jobs == nil {
		t.Fatal("missing jobs map")
	}
	requiredJobs := []string{
		"ci-gate",
		"golangci-lint",
		"security",
		"helm-lint",
		"trivy-images",
		"go",
		"integration-smoke",
		"integration-e2e",
	}
	for _, j := range requiredJobs {
		if _, ok := jobs[j]; !ok {
			t.Errorf("missing job %q", j)
		}
	}
	perms, ok := root["permissions"].(map[string]interface{})
	if !ok || perms == nil {
		t.Fatal("missing permissions map")
	}
	if _, ok := perms["pull-requests"]; !ok {
		t.Error(`permissions missing pull-requests (sticky coverage comment)`)
	}
}
