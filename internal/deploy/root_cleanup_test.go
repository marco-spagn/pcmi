package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrphanedScriptsRemoved asserts that the legacy root-level test scripts
// that were superseded by scripts in scripts/e2e/ no longer exist at the repo root.
func TestOrphanedScriptsRemoved(t *testing.T) {
	root := repoRoot(t)
	orphans := []string{
		"test_finale_v1.1.sh",
		"test_v1.5.sh",
		"test_embedding.sh",
	}
	for _, name := range orphans {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("orphaned script still exists at repo root: %s (delete it or move to scripts/e2e/legacy/)", path)
		}
	}
}

// TestQuickstartScriptExists asserts that scripts/quickstart.sh is present and executable.
func TestQuickstartScriptExists(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "scripts", "quickstart.sh")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("scripts/quickstart.sh not found: %v", err)
	}

	// Verify the file is executable by owner (mode bit 0100).
	if info.Mode()&0o100 == 0 {
		t.Errorf("scripts/quickstart.sh is not executable (mode: %s)", info.Mode())
	}
}

// TestQuickstartHasDockerCheck asserts that scripts/quickstart.sh contains
// dependency checks for both docker and jq.
func TestQuickstartHasDockerCheck(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "scripts", "quickstart.sh")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read scripts/quickstart.sh: %v", err)
	}
	content := string(data)

	checks := []string{"docker", "jq"}
	for _, dep := range checks {
		if !strings.Contains(content, dep) {
			t.Errorf("scripts/quickstart.sh is missing a dependency check for %q", dep)
		}
	}
}

// TestMakefileHasQuickstart asserts that the Makefile declares a "quickstart:" target.
func TestMakefileHasQuickstart(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "Makefile")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read Makefile: %v", err)
	}

	if !strings.Contains(string(data), "quickstart:") {
		t.Error("Makefile does not contain a 'quickstart:' target")
	}
}
