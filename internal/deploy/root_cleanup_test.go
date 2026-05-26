package deploy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot returns the absolute path to the repository root.
// It works reliably regardless of the current working directory or how `go test` is invoked.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller information")
	}

	// This file is at internal/deploy/root_cleanup_test.go
	// Go up 3 levels to reach the repo root.
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}
	return abs
}

func TestOrphanedScriptsAbsentAtRoot(t *testing.T) {
	root := repoRoot(t)

	orphaned := []string{
		"test_pcmi.sh",
		"test_finale_v1.1.sh",
		"test_v1.5.sh",
		"test_embedding.sh",
	}

	for _, name := range orphaned {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("orphaned script still present at root: %s", name)
		}
	}
}

func TestQuickstartScriptExistsAndExecutable(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "quickstart.sh")

	info, err := os.Stat(script)
	if err != nil {
		t.Fatalf("quickstart script not found: %v", err)
	}

	mode := info.Mode()
	if mode&0100 == 0 { // owner executable bit
		t.Error("quickstart.sh is not executable for owner")
	}
}

func TestQuickstartContainsDependencyChecks(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "quickstart.sh"))
	if err != nil {
		t.Fatalf("failed to read quickstart.sh: %v", err)
	}

	content := string(data)

	checks := []string{
		"docker",
		"jq",
	}

	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("quickstart.sh is missing dependency check for: %s", c)
		}
	}
}

func TestMakefileHasQuickstartTarget(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}

	content := string(data)

	// Flexible check for the target (handles comments, different formatting)
	if !strings.Contains(content, "quickstart:") {
		t.Error("Makefile is missing 'quickstart:' target")
	}
}
