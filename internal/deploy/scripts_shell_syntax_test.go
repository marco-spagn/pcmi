package deploy_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCriticalShellScriptsSyntaxOK runs `bash -n` on orchestration scripts so a
// typo doesn't surface only after merge.
func TestCriticalShellScriptsSyntaxOK(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	repo := repoRoot(t)
	for _, rel := range []string{
		"scripts/test_all_local.sh",
		"scripts/ci_coverage_check.sh",
		"scripts/infra_wait.sh",
		"scripts/ci_integration_smoke.sh",
	} {
		path := filepath.Join(repo, rel)
		cmd := exec.Command("bash", "-n", path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: bash -n failed: %v\n%s", rel, err, out)
		}
	}
}
