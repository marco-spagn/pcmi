package deploy_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestReleaseWorkflowYAMLValid guards the release automation workflow (PCMI-015).
func TestReleaseWorkflowYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/workflows/release.yml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse release.yml: %v", err)
	}
	name, _ := root["name"].(string)
	if name != "Release" {
		t.Errorf(`workflow name: got %q, want "Release"`, name)
	}
	jobs, ok := root["jobs"].(map[string]interface{})
	if !ok || jobs == nil {
		t.Fatal("missing jobs map")
	}
	if _, ok := jobs["release"]; !ok {
		t.Fatal(`missing jobs.release`)
	}
	text := string(data)
	for _, needle := range []string{
		"resolve_version.sh",
		"git-cliff",
		"cliff.toml",
		"softprops/action-gh-release",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("release.yml missing %q", needle)
		}
	}
}

// TestCliffTomlExists ensures git-cliff config is present for release automation.
func TestCliffTomlExists(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "cliff.toml"))
	if !strings.Contains(string(data), "conventional_commits = true") {
		t.Error("cliff.toml should enable conventional_commits")
	}
	if !strings.Contains(string(data), "feat") {
		t.Error("cliff.toml should define feat commit parser")
	}
}

// TestAPIVersioningDocExists links policy doc required by PCMI-015.
func TestAPIVersioningDocExists(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "docs/API-VERSIONING.md"))
	text := string(data)
	for _, needle := range []string{
		"internal/version/version.go",
		"Semantic Versioning",
		"resolve_version.sh",
		"cliff.toml",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("API-VERSIONING.md missing %q", needle)
		}
	}
}
