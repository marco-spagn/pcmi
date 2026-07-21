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

func TestCognitiveGraphTriggerDeletesStaleEdges(t *testing.T) {
	path := filepath.Join(repoRoot(t), "migrations", "019_cognitive_graph_age.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read 019_cognitive_graph_age.sql: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "delete_memory_link_from_graph") {
		t.Error("019 migration should define delete_memory_link_from_graph for stale AGE edge cleanup")
	}
	if !strings.Contains(body, "AFTER INSERT OR UPDATE OR DELETE") {
		t.Error("019 trigger should fire on DELETE to remove stale AGE edges")
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

func TestEntityGraphMigrationExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "migrations", "023_entity_graph.sql")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("migrations/023_entity_graph.sql not found: %v", err)
	}
}

func TestLinkProposalMigrationExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "migrations", "024_graph_link_proposals.sql")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("migrations/024_graph_link_proposals.sql not found: %v", err)
	}
}

func TestLinkProposalHandlerRegistered(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "handler", "link_proposal_handler.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read link_proposal_handler.go: %v", err)
	}
	body := string(data)
	for _, route := range []string{
		"/v1/graph/link-proposals",
		"/v1/graph/link-proposals/generate/:memory_id",
		"/v1/graph/link-proposals/:id/accept",
	} {
		if !strings.Contains(body, route) {
			t.Errorf("link_proposal_handler.go missing route %s", route)
		}
	}
}

func TestEntityGraphHandlerRegistered(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "handler", "graph_handler.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read graph_handler.go: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "/v1/graph/entities/memory") {
		t.Error("graph_handler.go does not register /v1/graph/entities/memory route")
	}
	if !strings.Contains(body, "/v1/graph/entities/related") {
		t.Error("graph_handler.go does not register /v1/graph/entities/related route")
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
	if !strings.Contains(body, "023_entity_graph.sql") {
		t.Error("docker-compose.yml default postgres service should mount 023_entity_graph.sql")
	}
	if !strings.Contains(body, "024_graph_link_proposals.sql") {
		t.Error("docker-compose.yml default postgres service should mount 024_graph_link_proposals.sql")
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

func TestCognitiveGraphMatrixDatasetExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "examples", "cognitive-graph-test-matrix", "graph_matrix.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("graph_matrix.json not found: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		`"causal"`,
		`"temporal"`,
		`"supports"`,
		`"contradicts"`,
		`"related"`,
		`"isolated"`,
		`"self_loop"`,
		`"cycle_a"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("graph_matrix.json missing %s case", want)
		}
	}
}

func TestCognitiveGraphMatrixScriptAndMakeTarget(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "e2e", "test_cognitive_graph_matrix.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("test_cognitive_graph_matrix.sh not found: %v", err)
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("could not read Makefile: %v", err)
	}
	if !strings.Contains(string(makefile), "test-cognitive-graph-matrix") {
		t.Error("Makefile should expose test-cognitive-graph-matrix target")
	}
}

func TestCognitiveGraphRealisticDatasetExists(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		filepath.Join("examples", "cognitive-graph-realistic", "README.md"),
		filepath.Join("examples", "cognitive-graph-realistic", "generate_realistic_graph.py"),
		filepath.Join("examples", "cognitive-graph-realistic", "smoke_load_to_pcmi.py"),
		filepath.Join("examples", "cognitive-graph-realistic", "validate_realistic_graph.py"),
		filepath.Join("examples", "cognitive-graph-realistic", "graph_realistic_large.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("%s not found: %v", rel, err)
		}
	}
}

func TestCognitiveGraphDatasetTestTargets(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		filepath.Join("examples", "soc-incident-graph", "test_loader.py"),
		filepath.Join("examples", "soc-incident-graph", "load_to_pcmi.py"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("%s not found: %v", rel, err)
		}
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("could not read Makefile: %v", err)
	}
	body := string(makefile)
	for _, want := range []string{"graph-realistic-smoke", "graph-soc-loader-test"} {
		if !strings.Contains(body, want) {
			t.Errorf("Makefile should expose %s target", want)
		}
	}
}

func TestCognitiveGraphRealisticDatasetCoverageMarkers(t *testing.T) {
	path := filepath.Join(repoRoot(t), "examples", "cognitive-graph-realistic", "graph_realistic_large.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read graph_realistic_large.json: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		`"node_count": 1200`,
		`"causal"`,
		`"temporal"`,
		`"supports"`,
		`"contradicts"`,
		`"related"`,
		`"kind": "campaign"`,
		`"kind": "evidence"`,
		`"kind": "hypothesis"`,
		`"kind": "postmortem"`,
		`"false_positive"`,
		`"benign_true_positive"`,
		`"duplicate"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("graph_realistic_large.json missing coverage marker %s", want)
		}
	}
}
