package deploy_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSDKPublishWorkflowValid verifies publish-sdks.yml is valid YAML and
// triggers on the release event.
func TestSDKPublishWorkflowValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, ".github/workflows/publish-sdks.yml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse publish-sdks.yml: %v", err)
	}

	name, _ := root["name"].(string)
	if name != "Publish SDKs" {
		t.Errorf(`workflow name: got %q, want "Publish SDKs"`, name)
	}

	// Verify release trigger.
	on, ok := root["on"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'on' trigger map")
	}
	if _, ok := on["release"]; !ok {
		t.Error("publish-sdks.yml missing on.release trigger")
	}
	if _, ok := on["workflow_dispatch"]; !ok {
		t.Error("publish-sdks.yml missing on.workflow_dispatch trigger")
	}

	jobs, ok := root["jobs"].(map[string]interface{})
	if !ok {
		t.Fatal("missing jobs map")
	}
	for _, job := range []string{"publish-python", "publish-npm", "tag-go-sdk"} {
		if _, ok := jobs[job]; !ok {
			t.Errorf("missing jobs.%s", job)
		}
	}

	text := string(data)
	for _, needle := range []string{
		"PYPI_API_TOKEN",
		"NPM_TOKEN",
		"twine",
		"npm publish",
		"sdk/go/v",
		"pyproject.toml",
		"package.json",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("publish-sdks.yml missing %q", needle)
		}
	}
}

// TestSDKPythonPyprojectValid checks pyproject.toml has [build-system] and
// name = "pcmi".
func TestSDKPythonPyprojectValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "sdk/python/pyproject.toml"))
	text := string(data)

	if !strings.Contains(text, `name = "pcmi"`) {
		t.Error(`sdk/python/pyproject.toml missing name = "pcmi"`)
	}
	if !strings.Contains(text, "[build-system]") {
		t.Error("sdk/python/pyproject.toml missing [build-system] section")
	}
	if !strings.Contains(text, "hatchling") {
		t.Error("sdk/python/pyproject.toml build-system should use hatchling")
	}
	if !strings.Contains(text, "Programming Language :: Python :: 3.12") {
		t.Error("sdk/python/pyproject.toml missing Python 3.12 classifier")
	}
	if !strings.Contains(text, "License :: OSI Approved :: MIT License") {
		t.Error("sdk/python/pyproject.toml missing MIT License classifier")
	}
}

// TestSDKTypescriptPackageJSONValid checks package.json has required fields.
func TestSDKTypescriptPackageJSONValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "sdk/typescript/package.json"))

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse sdk/typescript/package.json: %v", err)
	}

	if pkg["main"] == nil {
		t.Error(`package.json missing "main" field`)
	}
	if pkg["types"] == nil {
		t.Error(`package.json missing "types" field`)
	}
	if pkg["files"] == nil {
		t.Error(`package.json missing "files" field`)
	}

	scripts, _ := pkg["scripts"].(map[string]interface{})
	if scripts == nil {
		t.Fatal(`package.json missing "scripts" object`)
	}
	if scripts["build"] == nil {
		t.Error(`package.json missing scripts.build`)
	}
	if scripts["prepublish"] == nil {
		t.Error(`package.json missing scripts.prepublish`)
	}
}

// TestSDKGoModulePath verifies sdk/go/go.mod has the canonical module path.
func TestSDKGoModulePath(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "sdk/go/go.mod"))
	text := string(data)

	const want = "module github.com/marco-spagn/pcmi/sdk/go"
	if !strings.Contains(text, want) {
		t.Errorf("sdk/go/go.mod: expected module path %q", want)
	}
}
