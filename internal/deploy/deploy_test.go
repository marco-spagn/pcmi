// Package deploy_test validates the contents of deploy/ artifacts (Grafana
// dashboard, Prometheus alert rules, Dockerfiles) so a broken file is caught
// in CI before it reaches a cluster.
//
// These tests deliberately live in their own package with no production code
// — the deploy/ directory is data, not Go source.
package deploy_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// repoRoot returns the absolute path of the repository root by walking up
// from the test file location until go.mod is found.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find repo root (no go.mod up the tree)")
	return ""
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatalf("file %s is empty", path)
	}
	return data
}

// ─── Grafana dashboard ────────────────────────────────────────────────────────

func TestGrafanaDashboardIsValidJSON(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "grafana", "pcmi-dashboard.json")
	data := readFile(t, path)

	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("dashboard is not valid JSON: %v", err)
	}
}

func TestGrafanaDashboardHasRequiredFields(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "grafana", "pcmi-dashboard.json")
	var dashboard map[string]any
	if err := json.Unmarshal(readFile(t, path), &dashboard); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"title", "panels", "schemaVersion", "uid", "tags"} {
		if _, ok := dashboard[key]; !ok {
			t.Errorf("dashboard missing required field %q", key)
		}
	}

	if title, _ := dashboard["title"].(string); !strings.Contains(strings.ToLower(title), "pcmi") {
		t.Errorf(`dashboard title %q does not mention "pcmi"`, title)
	}

	panels, ok := dashboard["panels"].([]any)
	if !ok {
		t.Fatal("dashboard.panels is not an array")
	}
	if len(panels) < 5 {
		t.Errorf("expected at least 5 panels, got %d", len(panels))
	}
}

func TestGrafanaDashboardReferencesExpectedMetrics(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "grafana", "pcmi-dashboard.json")
	data := readFile(t, path)
	body := string(data)

	// All currently-exported PCMI metrics that we expect the dashboard to use.
	required := []string{
		"pcmi_memory_stores_total",
		"pcmi_memory_retrieves_total",
		"pcmi_worker_redis_events_total",
		"pcmi_distillation_total",
		"pcmi_distillation_duration_seconds",
		"pcmi_distillation_sources_per_job",
		"pcmi_distillation_active_jobs",
		"pcmi_distillation_queued_jobs",
	}
	for _, m := range required {
		if !strings.Contains(body, m) {
			t.Errorf("dashboard does not reference metric %q", m)
		}
	}
}

func TestGrafanaDashboardEveryPanelHasTarget(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "grafana", "pcmi-dashboard.json")
	var dashboard map[string]any
	if err := json.Unmarshal(readFile(t, path), &dashboard); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	panels := dashboard["panels"].([]any)
	for i, p := range panels {
		panel := p.(map[string]any)
		if t, _ := panel["type"].(string); t == "row" {
			continue // section dividers have no targets
		}
		targets, ok := panel["targets"].([]any)
		if !ok || len(targets) == 0 {
			t.Errorf("panel %d (%q) has no targets", i, panel["title"])
			continue
		}
		for j, tg := range targets {
			tgm, _ := tg.(map[string]any)
			if expr, _ := tgm["expr"].(string); strings.TrimSpace(expr) == "" {
				t.Errorf("panel %d target %d has empty expr", i, j)
			}
		}
	}
}

// ─── Prometheus alert rules ───────────────────────────────────────────────────

type promRulesFile struct {
	Groups []promGroup `yaml:"groups"`
}

type promGroup struct {
	Name     string     `yaml:"name"`
	Interval string     `yaml:"interval,omitempty"`
	Rules    []promRule `yaml:"rules"`
}

type promRule struct {
	Alert       string            `yaml:"alert,omitempty"`
	Record      string            `yaml:"record,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func TestPrometheusAlertsIsValidYAML(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "prometheus", "alerts.yaml")
	data := readFile(t, path)

	var rules promRulesFile
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatalf("alerts.yaml is not valid YAML: %v", err)
	}
	if len(rules.Groups) == 0 {
		t.Fatal("alerts.yaml has no groups")
	}
}

func TestPrometheusAlertsAllRulesHaveRequiredFields(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "prometheus", "alerts.yaml")
	var rules promRulesFile
	if err := yaml.Unmarshal(readFile(t, path), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var errs []error
	for _, g := range rules.Groups {
		if g.Name == "" {
			errs = append(errs, errors.New("group with empty name"))
		}
		if len(g.Rules) == 0 {
			errs = append(errs, errors.New("group "+g.Name+" has no rules"))
		}
		for i, r := range g.Rules {
			if r.Alert == "" && r.Record == "" {
				errs = append(errs, errors.New(g.Name+" rule "+itoa(i)+" missing alert/record name"))
			}
			if strings.TrimSpace(r.Expr) == "" {
				errs = append(errs, errors.New(g.Name+"/"+r.Alert+" has empty expr"))
			}
			if r.Alert != "" {
				if r.Labels["severity"] == "" {
					errs = append(errs, errors.New(g.Name+"/"+r.Alert+" missing severity label"))
				}
				if r.Annotations["summary"] == "" {
					errs = append(errs, errors.New(g.Name+"/"+r.Alert+" missing summary annotation"))
				}
			}
		}
	}
	for _, e := range errs {
		t.Error(e)
	}
}

func TestPrometheusAlertsReferenceKnownMetrics(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "prometheus", "alerts.yaml")
	var rules promRulesFile
	if err := yaml.Unmarshal(readFile(t, path), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	knownMetrics := []string{
		"pcmi_memory_stores_total",
		"pcmi_memory_retrieves_total",
		"pcmi_worker_redis_events_total",
		"pcmi_distillation_total",
		"pcmi_distillation_duration_seconds",
		"pcmi_distillation_sources_per_job",
		"pcmi_distillation_active_jobs",
		"pcmi_distillation_queued_jobs",
	}
	hasKnown := func(expr string) bool {
		for _, m := range knownMetrics {
			if strings.Contains(expr, m) {
				return true
			}
		}
		return false
	}

	for _, g := range rules.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				continue
			}
			if !hasKnown(r.Expr) {
				t.Errorf("alert %s/%s does not reference any known pcmi_* metric: %q",
					g.Name, r.Alert, r.Expr)
			}
		}
	}
}

func TestPrometheusAlertsSeverityValues(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "prometheus", "alerts.yaml")
	var rules promRulesFile
	if err := yaml.Unmarshal(readFile(t, path), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	allowed := map[string]bool{"info": true, "warning": true, "critical": true}
	for _, g := range rules.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				continue
			}
			sev := r.Labels["severity"]
			if !allowed[sev] {
				t.Errorf("alert %s/%s has invalid severity %q (allowed: info|warning|critical)",
					g.Name, r.Alert, sev)
			}
		}
	}
}

// ─── Dockerfiles ──────────────────────────────────────────────────────────────

func TestDockerfilesExist(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"Dockerfile.api", "Dockerfile.worker", "Dockerfile"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s missing or unreadable: %v", name, err)
		}
	}
}

func TestDockerfileAPIBuildsOnlyAPI(t *testing.T) {
	path := filepath.Join(repoRoot(t), "Dockerfile.api")
	body := string(readFile(t, path))

	// Must build pcmi-api
	if !strings.Contains(body, "go build") || !strings.Contains(body, "./cmd/api") {
		t.Error("Dockerfile.api does not appear to build ./cmd/api")
	}
	// Must NOT build the worker
	if strings.Contains(body, "./cmd/worker") {
		t.Error("Dockerfile.api also builds ./cmd/worker — should be API-only")
	}
	// Must run as non-root (security hardening)
	if !strings.Contains(body, "USER pcmi") {
		t.Error("Dockerfile.api does not switch to non-root user")
	}
	// Entrypoint should be the API binary
	if !strings.Contains(body, "pcmi-api") {
		t.Error("Dockerfile.api does not reference pcmi-api binary")
	}
}

func TestDockerfileWorkerBuildsOnlyWorker(t *testing.T) {
	path := filepath.Join(repoRoot(t), "Dockerfile.worker")
	body := string(readFile(t, path))

	if !strings.Contains(body, "go build") || !strings.Contains(body, "./cmd/worker") {
		t.Error("Dockerfile.worker does not appear to build ./cmd/worker")
	}
	if strings.Contains(body, "./cmd/api") {
		t.Error("Dockerfile.worker also builds ./cmd/api — should be worker-only")
	}
	if !strings.Contains(body, "USER pcmi") {
		t.Error("Dockerfile.worker does not switch to non-root user")
	}
	if !strings.Contains(body, "pcmi-worker") {
		t.Error("Dockerfile.worker does not reference pcmi-worker binary")
	}
}

func TestDockerComposeUsesSeparateDockerfiles(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docker-compose.yml")
	body := string(readFile(t, path))

	if !strings.Contains(body, "Dockerfile.api") {
		t.Error("docker-compose.yml does not reference Dockerfile.api")
	}
	if !strings.Contains(body, "Dockerfile.worker") {
		t.Error("docker-compose.yml does not reference Dockerfile.worker")
	}
}

// TestCITrivyActionTagFormat guards two things at once:
//
//  1. If the workflow uses the trivy-action wrapper (`uses: aquasecurity/...`),
//     every pin must be either a v-prefixed tag (e.g. v0.28.0) or a full
//     40-char commit SHA. A bare `0.28.0` was the original CI breakage.
//  2. If the workflow no longer uses the wrapper (current setup: it runs
//     `aquasec/trivy:latest` via `docker run`), the test makes sure the
//     direct-docker form is still present, so nobody silently drops Trivy.
//
// To keep the scanner unambiguous, only `uses:` lines are considered for the
// wrapper check — comments mentioning the wrapper as plain text are ignored.
func TestCITrivyActionTagFormat(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")
	body := string(readFile(t, path))

	const marker = "uses: aquasecurity/trivy-action@"
	idx := 0
	found := 0
	for {
		off := strings.Index(body[idx:], marker)
		if off < 0 {
			break
		}
		start := idx + off + len(marker)
		// Read until newline or whitespace.
		end := start
		for end < len(body) && body[end] != '\n' && body[end] != ' ' && body[end] != '\r' {
			end++
		}
		ref := strings.TrimSpace(body[start:end])
		if ref == "" {
			t.Errorf("empty trivy-action ref at offset %d", start)
		}
		// Must be either a v-prefixed tag or a full SHA (40 hex chars).
		if !strings.HasPrefix(ref, "v") && len(ref) != 40 {
			t.Errorf("trivy-action pinned to %q — expected v-prefixed tag (e.g. v0.28.0) or full commit SHA", ref)
		}
		found++
		idx = end
	}
	if found == 0 {
		// Workflow does not use the wrapper. Make sure the direct-docker
		// equivalent is in place so Trivy scanning isn't quietly disabled.
		if !strings.Contains(body, "aquasec/trivy:latest") {
			t.Error("workflow has no aquasecurity/trivy-action `uses:` AND no `aquasec/trivy:latest` docker invocation — Trivy scanning seems disabled")
		}
	}
}

// itoa is a tiny strconv replacement to keep imports minimal.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
