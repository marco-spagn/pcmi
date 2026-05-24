package deploy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCIWorkflowYAMLValid guards the main GitHub Actions workflow: broken YAML
// or dropped jobs surface before merge.
func TestCIWorkflowYAMLValid(t *testing.T) {
	repo := repoRoot(t)
	ciPath := filepath.Join(repo, ".github/workflows/ci.yml")
	data := readFile(t, ciPath)

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	name, _ := root["name"].(string)
	if name != "CI" {
		t.Errorf(`workflow name: got %q, want "CI"`, name)
	}
	jobs, ok := root["jobs"].(map[string]interface{})
	if !ok || jobs == nil {
		t.Fatal("missing jobs map")
	}
	requiredJobs := []string{
		"golangci-lint",
		"security",
		"helm-lint",
		"trivy-images",
		"go",
		"integration-smoke",
		"integration-e2e",
	}
	for _, j := range requiredJobs {
		if _, ok := jobs[j]; !ok {
			t.Errorf("missing job %q", j)
		}
	}

	// Job DAG expectations (needs edges).
	smoke, _ := jobs["integration-smoke"].(map[string]interface{})
	if smoke != nil {
		needs := stringSliceField(t, smoke, "needs")
		for _, want := range []string{"go", "golangci-lint", "security", "trivy-images"} {
			if !containsString(needs, want) {
				t.Errorf("integration-smoke needs: missing %q (got %v)", want, needs)
			}
		}
	}
	e2e, _ := jobs["integration-e2e"].(map[string]interface{})
	if e2e != nil {
		needs := stringSliceField(t, e2e, "needs")
		for _, want := range []string{"go", "golangci-lint"} {
			if !containsString(needs, want) {
				t.Errorf("integration-e2e needs: missing %q (got %v)", want, needs)
			}
		}
	}

	perms, ok := root["permissions"].(map[string]interface{})
	if !ok || perms == nil {
		t.Fatal("missing permissions map")
	}
	if _, ok := perms["pull-requests"]; !ok {
		t.Error(`permissions missing pull-requests (sticky coverage comment)`)
	}

	conc, ok := root["concurrency"].(map[string]interface{})
	if !ok || conc == nil {
		t.Fatal("missing concurrency block (cancel in-progress on new push)")
	}
	if conc["cancel-in-progress"] != true {
		t.Error("concurrency.cancel-in-progress should be true")
	}

	for _, j := range requiredJobs {
		job, _ := jobs[j].(map[string]interface{})
		if job == nil {
			continue
		}
		if _, has := job["timeout-minutes"]; !has {
			t.Errorf("job %q missing timeout-minutes", j)
		}
	}

	ciText := string(data)
	if regexp.MustCompile(`PCMI_EXPECT_VERSION:\s*v[0-9]`).MatchString(ciText) {
		t.Error("ci.yml must not hardcode PCMI_EXPECT_VERSION; use pcmi-resolve-version action")
	}
	if !strings.Contains(ciText, "scripts/ci/resolve_version.sh") &&
		!strings.Contains(ciText, "pcmi-resolve-version") {
		t.Error("ci.yml should reference version resolution (action or script)")
	}
	if !strings.Contains(ciText, "Job DAG") {
		t.Error("ci.yml should document job DAG in header comments")
	}
	if !strings.Contains(ciText, "git push origin HEAD:main") {
		t.Error("ci.yml should push coverage badge updates directly to main")
	}
	if !strings.Contains(ciText, "persist-credentials: true") {
		t.Error("ci.yml go job should use actions/checkout with persist-credentials for badge push")
	}
	if strings.Contains(ciText, "peter-evans/create-pull-request") {
		t.Error("ci.yml must not use create-pull-request for badge updates (direct push to main)")
	}
	if !strings.Contains(ciText, "badges/**") {
		t.Error("ci.yml should paths-ignore badges/** on push to avoid badge-update CI loops")
	}

	resolveScript := filepath.Join(repo, "scripts/ci/resolve_version.sh")
	if _, err := os.Stat(resolveScript); err != nil {
		t.Errorf("missing %s: %v", resolveScript, err)
	}
}

func stringSliceField(t *testing.T, m map[string]interface{}, key string) []string {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, _ := item.(string)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	default:
		t.Errorf("%s: unexpected type %T", key, raw)
		return nil
	}
}

func containsString(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
