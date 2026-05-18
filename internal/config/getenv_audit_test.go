package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestNoOSGetenvOutsideConfig is the structural invariant locked in by PR #2:
//
// Production code (cmd/* and internal/*, excluding internal/config) MUST NOT
// call os.Getenv directly. All runtime configuration goes through
// *config.Config — single source of truth, validated at startup, documented
// in .env.example.
//
// This test walks the source tree and fails CI the moment a new env read
// sneaks in. Allowed exceptions:
//
//   - any *_test.go file (tests legitimately need env knobs)
//   - internal/config/** (the config package itself defines Load())
//   - scripts/** (standalone CLI tools, not deployed binaries)
//
// When you genuinely need a new runtime knob:
//  1. add the field to internal/config/config.go,
//  2. document it in .env.example,
//  3. read cfg.X at the call site.
func TestNoOSGetenvOutsideConfig(t *testing.T) {
	repo := walkRepoRoot(t)

	getenvRe := regexp.MustCompile(`\bos\.Getenv\b`)
	var violations []string

	roots := []string{
		filepath.Join(repo, "cmd"),
		filepath.Join(repo, "internal"),
	}

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// If the directory doesn't exist (e.g. cmd/ removed), skip silently.
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// Anything under internal/config is the canonical config layer.
			rel, _ := filepath.Rel(repo, path)
			if strings.HasPrefix(rel, "internal/config"+string(filepath.Separator)) {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if getenvRe.Match(data) {
				violations = append(violations, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	if len(violations) == 0 {
		return
	}
	t.Fatalf(
		"os.Getenv is forbidden in production code outside internal/config.\n"+
			"Found %d violation(s):\n  - %s\n\n"+
			"Fix: add a field to internal/config/config.go, document it in "+
			".env.example, and read cfg.X at the call site.",
		len(violations), strings.Join(violations, "\n  - "),
	)
}

// walkRepoRoot climbs up from this test file until it finds go.mod, so the
// test is location-independent and survives `go test ./internal/config/...`.
func walkRepoRoot(t *testing.T) string {
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
	t.Fatal("could not locate repo root (no go.mod up the tree)")
	return ""
}
