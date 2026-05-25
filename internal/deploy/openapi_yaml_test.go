package deploy_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPIYAMLValid locks docs/openapi.yaml to OpenAPI 3.x with core routes.
func TestOpenAPIYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "docs", "openapi.yaml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("openapi.yaml parse: %v", err)
	}
	ver, _ := root["openapi"].(string)
	if ver == "" || !strings.HasPrefix(ver, "3.") {
		t.Fatalf(`openapi field: want 3.x semver string, got %q`, ver)
	}
	paths, ok := root["paths"].(map[string]interface{})
	if !ok || paths == nil {
		t.Fatal("missing paths map")
	}
	for _, p := range []string{
		"/health",
		"/v1/health",
		"/v1/memories/refine",
		"/v1/memories/links",
		"/v1/lineage/memory",
		"/v1/lineage/distilled/{id}",
		"/v1/webhooks",
	} {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %q", p)
		}
	}
	info, ok := root["info"].(map[string]interface{})
	if !ok {
		t.Fatal("missing info block")
	}
	if title, _ := info["title"].(string); !strings.Contains(strings.ToLower(title), "pcmi") {
		t.Errorf("info.title should mention PCMI, got %q", title)
	}
}
