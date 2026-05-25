package deploy_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestReleaseWorkflowYAMLValid guards the release automation workflow.
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
	for _, job := range []string{"release-binaries", "release-container"} {
		if _, ok := jobs[job]; !ok {
			t.Errorf("missing jobs.%s", job)
		}
	}

	text := string(data)
	for _, needle := range []string{
		"resolve_version.sh",
		"git-cliff",
		"cliff.toml",
		"softprops/action-gh-release",
		"ghcr.io/marco-spagn/pcmi",
		"docker/build-push-action",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("release.yml missing %q", needle)
		}
	}
}

// TestReleaseWorkflowTriggersOnTags verifies the workflow fires on v* tags.
func TestReleaseWorkflowTriggersOnTags(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/workflows/release.yml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse release.yml: %v", err)
	}

	on, ok := root["on"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'on' trigger map")
	}
	push, ok := on["push"].(map[string]interface{})
	if !ok {
		t.Fatal("missing on.push")
	}
	tags, ok := push["tags"].([]interface{})
	if !ok || len(tags) == 0 {
		t.Fatal("missing on.push.tags")
	}
	found := false
	for _, tag := range tags {
		s, _ := tag.(string)
		if strings.HasPrefix(s, "v") {
			found = true
			break
		}
	}
	if !found {
		t.Error("on.push.tags does not contain a v* pattern")
	}
}

// TestReleaseWorkflowContainerJob verifies the container job targets ghcr.io.
func TestReleaseWorkflowContainerJob(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/workflows/release.yml"))
	text := string(data)

	for _, needle := range []string{
		"ghcr.io",
		"linux/amd64,linux/arm64",
		"docker/login-action",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("release-container job missing %q", needle)
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

// TestAPIVersioningDocExists links policy doc required by the release pipeline.
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
