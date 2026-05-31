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

func TestCognitiveGraphRoutesAlwaysRegistered(t *testing.T) {
	path := filepath.Join(repoRoot(t), "cmd", "api", "main.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read cmd/api/main.go: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "handler.RegisterGraphRoutes(app, graphClient)") {
		t.Error("cmd/api/main.go must call handler.RegisterGraphRoutes unconditionally")
	}
	if strings.Contains(body, "if graphClient.IsAvailable(ctx) {\n\t\thandler.RegisterGraphRoutes") {
		t.Error("RegisterGraphRoutes must not be gated on IsAvailable at startup")
	}
}

func TestCognitiveGraphTriggerSyncsOnUpdate(t *testing.T) {
	path := filepath.Join(repoRoot(t), "migrations", "019_cognitive_graph_age.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read 019_cognitive_graph_age.sql: %v", err)
	}
	if !strings.Contains(string(data), "AFTER INSERT OR UPDATE") {
		t.Error("019 trigger should fire on INSERT OR UPDATE for upserted memory_links")
	}
}

func TestCognitiveGraphDockerfileExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docker", "postgres-age", "Dockerfile.postgres-age")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("docker/postgres-age/Dockerfile.postgres-age not found: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "pgvector/pgvector:pg16") {
		t.Error("Dockerfile.postgres-age must be based on pgvector/pgvector:pg16")
	}
	if !strings.Contains(body, "apache/age") {
		t.Error("Dockerfile.postgres-age must build Apache AGE")
	}
}

func TestCognitiveGraphMigrationMountedDefaultPostgres(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docker-compose.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read docker-compose.yml: %v", err)
	}
	body := string(data)
	// The default postgres service should mount 019 so the migration is applied
	// even without the graph profile. The migration degrades gracefully when AGE is missing.
	if !strings.Contains(body, "019_cognitive_graph_age.sql") {
		t.Error("docker-compose.yml default postgres service should mount 019_cognitive_graph_age.sql")
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
	if !strings.Contains(body, "Dockerfile.postgres-age") {
		t.Error("docker-compose.yml postgres-age should use the custom Dockerfile.postgres-age image")
	}
}
