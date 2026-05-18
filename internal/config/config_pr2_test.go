package config

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Regression tests for configuration; env-drift guard lives in TestEnvExampleStaysInSyncWithConfig.

func TestLoadRateLimitRPMOverride(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM", "30")
	cfg := Load()
	if cfg.RateLimitRPM != 30 {
		t.Fatalf("expected override 30, got %d", cfg.RateLimitRPM)
	}
}

// TestEnvExampleStaysInSyncWithConfig parses .env.example, extracts every VAR= name,
// and asserts that config.go references each name (os.Getenv / envOr / envInt / envBool).
func TestEnvExampleStaysInSyncWithConfig(t *testing.T) {
	repo := repoRoot(t)

	envExamplePath := filepath.Join(repo, ".env.example")
	configGoPath := filepath.Join(repo, "internal", "config", "config.go")

	exampleVars := parseEnvExample(t, envExamplePath)
	if len(exampleVars) == 0 {
		t.Fatal(".env.example: parsed zero variables — parser is broken")
	}

	cfgSrc, err := os.ReadFile(configGoPath)
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}
	cfgText := string(cfgSrc)

	for _, v := range exampleVars {
		if !strings.Contains(cfgText, `"`+v+`"`) {
			t.Errorf(".env.example documents %q but config.go does not reference it", v)
		}
	}
}

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
	t.Fatal("could not locate repo root (no go.mod up the tree)")
	return ""
}

var envVarRe = regexp.MustCompile(`^\s*#?\s*([A-Z][A-Z0-9_]*)=`)

func parseEnvExample(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	seen := map[string]struct{}{}
	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := envVarRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		name := m[1]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}
