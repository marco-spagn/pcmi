package deploy_test

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDockerComposeYAMLValid ensures docker-compose.yml parses and declares the
// minimal runtime topology PCMI documents (compose is the default dev path).
func TestDockerComposeYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, "docker-compose.yml"))

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("docker-compose.yml parse: %v", err)
	}
	services, ok := root["services"].(map[string]interface{})
	if !ok || services == nil {
		t.Fatal("missing services map")
	}
	for _, name := range []string{"postgres", "redis", "api", "worker"} {
		if _, ok := services[name]; !ok {
			t.Errorf("missing service %q", name)
		}
	}
}
