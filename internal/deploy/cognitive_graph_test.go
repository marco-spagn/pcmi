package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCognitiveGraphMigrationExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "migrations", "019_cognitive_graph_age.sql")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("migrations/019_cognitive_graph_age.sql not found: %v", err)
	}
}

func TestCognitiveGraphDocExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "cognitive-graph.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("docs/cognitive-graph.md not found: %v", err)
	}
}

func TestCognitiveGraphHandlerRegistered(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "handler", "graph_handler.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read graph_handler.go: %v", err)
	}
	if !strings.Contains(string(data), "/v1/graph/related") {
		t.Error("graph_handler.go does not register /v1/graph/related route")
	}
}

func TestCognitiveGraphGracefulDegradation(t *testing.T) {
	path := filepath.Join(repoRoot(t), "migrations", "019_cognitive_graph_age.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read 019_cognitive_graph_age.sql: %v", err)
	}
	if !strings.Contains(string(data), "EXCEPTION WHEN") {
		t.Error("019_cognitive_graph_age.sql lacks EXCEPTION WHEN block for graceful degradation")
	}
}

func TestCognitiveGraphDocMentionsDockerCompose(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "cognitive-graph.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read cognitive-graph.md: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "docker compose") {
		t.Error("cognitive-graph.md should mention docker compose startup")
	}
}

func TestCognitiveGraphDockerComposeProfile(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docker-compose.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read docker-compose.yml: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "postgres-age") {
		t.Error("docker-compose.yml does not define postgres-age service")
	}
	if !strings.Contains(body, `profiles: ["graph"]`) {
		t.Error(`docker-compose.yml postgres-age service should have profiles: ["graph"]`)
	}
}
