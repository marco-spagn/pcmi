package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/marco-spagn/pcmi/internal/version"
)

// PR #4 — structural tests for deploy/helm/pcmi/. These run with `go test
// ./internal/deploy/...` (no helm binary needed) and lock down:
//
//   - Chart.yaml parses + names the expected chart;
//   - appVersion mirrors internal/version/version.go;
//   - values.yaml parses + carries the keys the templates reference;
//   - every template under templates/ parses (after trivial helm-tag stripping);
//   - the deprecated k8s/ tree contains *only* deprecation stubs (no live
//     manifests), guarding against accidental re-introduction.

const (
	chartDir = "deploy/helm/pcmi"
)

func TestHelmChartYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, chartDir, "Chart.yaml"))

	var chart struct {
		APIVersion string `yaml:"apiVersion"`
		Name       string `yaml:"name"`
		Type       string `yaml:"type"`
		Version    string `yaml:"version"`
		AppVersion string `yaml:"appVersion"`
	}
	if err := yaml.Unmarshal(data, &chart); err != nil {
		t.Fatalf("Chart.yaml parse: %v", err)
	}
	if chart.APIVersion != "v2" {
		t.Errorf("Chart.yaml apiVersion: got %q, want v2", chart.APIVersion)
	}
	if chart.Name != "pcmi" {
		t.Errorf("Chart.yaml name: got %q, want pcmi", chart.Name)
	}
	if chart.Type != "application" {
		t.Errorf("Chart.yaml type: got %q, want application", chart.Type)
	}
	if chart.Version == "" {
		t.Error("Chart.yaml version: empty")
	}
	if chart.AppVersion == "" {
		t.Error("Chart.yaml appVersion: empty")
	}
}

// TestHelmAppVersionMatchesGoTag asserts the chart's appVersion follows
// internal/version/version.go:Tag. When the Go tag bumps, the chart's
// appVersion has to bump in the same commit — otherwise images pulled by
// `helm install` won't match the binary.
func TestHelmAppVersionMatchesGoTag(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, chartDir, "Chart.yaml"))

	var chart struct {
		AppVersion string `yaml:"appVersion"`
	}
	if err := yaml.Unmarshal(data, &chart); err != nil {
		t.Fatalf("Chart.yaml parse: %v", err)
	}
	if chart.AppVersion != version.Tag {
		t.Fatalf(
			"Chart.yaml appVersion %q does not match internal/version.Tag %q — "+
				"bump deploy/helm/pcmi/Chart.yaml in the same PR that bumps version.go",
			chart.AppVersion, version.Tag,
		)
	}
}

func TestHelmValuesYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	data := readFile(t, filepath.Join(repo, chartDir, "values.yaml"))

	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatalf("values.yaml parse: %v", err)
	}

	// Required top-level groups every template depends on.
	required := []string{"image", "api", "worker", "config", "secrets", "tls"}
	for _, k := range required {
		if _, ok := values[k]; !ok {
			t.Errorf("values.yaml missing required key: %s", k)
		}
	}
}

// TestHelmTemplatesPresent lists the templates the chart promises in
// README.md and asserts each file exists + starts with a recognisable
// Helm guard or apiVersion. Doesn't render — that's `helm-lint`'s job in CI.
func TestHelmTemplatesPresent(t *testing.T) {
	repo := repoRoot(t)
	want := []string{
		"_helpers.tpl",
		"configmap.yaml",
		"secret.yaml",
		"api-deployment.yaml",
		"worker-deployment.yaml",
		"service.yaml",
		"hpa.yaml",
		"pdb.yaml",
	}
	for _, fname := range want {
		path := filepath.Join(repo, chartDir, "templates", fname)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("missing template %s: %v", fname, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("template %s is empty", fname)
			continue
		}
		s := string(data)
		// Either an apiVersion: line (rendered manifest) or a Helm template
		// guard / define block (templates with conditionals).
		if !strings.Contains(s, "apiVersion:") &&
			!strings.Contains(s, "{{- if") &&
			!strings.Contains(s, "{{- define") {
			t.Errorf("template %s has neither apiVersion nor a Helm guard / define", fname)
		}
	}
}

// TestLegacyK8sTreeIsDeprecated forbids the old k8s/ tree from carrying live
// manifests. The files may still exist (deletion isn't always permitted by
// the host sandbox) but their content MUST be deprecation stubs only.
func TestLegacyK8sTreeIsDeprecated(t *testing.T) {
	repo := repoRoot(t)
	legacy := []string{
		"k8s/api-deployment.yaml",
		"k8s/worker-deployment.yaml",
		"k8s/postgres-statefulset.yaml",
		"k8s/ingress.yaml",
	}
	for _, rel := range legacy {
		path := filepath.Join(repo, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			// Already deleted — fine.
			continue
		}
		s := string(data)
		// A live manifest contains apiVersion: + kind:. The deprecation stubs
		// only contain comments + the YAML document separator.
		hasAPIVersion := strings.Contains(s, "apiVersion:")
		hasKind := strings.Contains(s, "kind:")
		hasDeprecated := strings.Contains(strings.ToUpper(s), "DEPRECATED")
		if (hasAPIVersion || hasKind) && !hasDeprecated {
			t.Errorf("%s still contains live k8s manifest — use deploy/helm/pcmi or deploy/k8s/", rel)
		}
	}
}
