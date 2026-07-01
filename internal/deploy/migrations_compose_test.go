package deploy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestComposeReferencesExistingMigrations asserts every ./migrations/*.sql bind
// mount declared in docker-compose.yml points at a real file (catches renames).
func TestComposeReferencesExistingMigrations(t *testing.T) {
	repo := repoRoot(t)
	composePath := filepath.Join(repo, "docker-compose.yml")
	raw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	const prefix = "./migrations/"
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, prefix) || !strings.Contains(line, ".sql") {
			continue
		}
		// Typical: - ./migrations/001_init.sql:/docker-entrypoint-initdb.d/...
		part := strings.TrimSpace(strings.Split(line, ":")[0])
		part = strings.TrimPrefix(part, "- ")
		part = strings.Trim(part, `"'`)
		if !strings.HasPrefix(part, prefix) {
			continue
		}
		full := filepath.Join(repo, filepath.FromSlash(strings.TrimPrefix(part, "./")))
		st, err := os.Stat(full)
		if err != nil || st.IsDir() {
			t.Errorf("compose references missing migration file %q", part)
		}
	}
}

// serviceHeaderRE matches a top-level compose service key (exactly two-space
// indent), e.g. "  postgres:" or "  postgres-age:".
var serviceHeaderRE = regexp.MustCompile(`^  ([a-z][a-z0-9_-]*):\s*$`)

// mountedMigrations returns the set of migration file basenames bind-mounted into
// /docker-entrypoint-initdb.d for a given compose service.
func mountedMigrations(body, service string) map[string]bool {
	out := map[string]bool{}
	inBlock := false
	for _, line := range strings.Split(body, "\n") {
		if m := serviceHeaderRE.FindStringSubmatch(line); m != nil {
			inBlock = m[1] == service
			continue
		}
		if len(line) > 0 && line[0] != ' ' { // a top-level key ends the services block
			inBlock = false
		}
		if !inBlock {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "./migrations/") || !strings.Contains(trimmed, ".sql") {
			continue
		}
		part := strings.TrimSpace(strings.Split(trimmed, ":")[0])
		part = strings.TrimPrefix(part, "- ")
		part = strings.Trim(part, `"'`)
		if strings.HasPrefix(part, "./migrations/") {
			out[filepath.Base(part)] = true
		}
	}
	return out
}

// TestEveryMigrationMountedInInitdbServices is the reverse of the check above: it
// asserts that EVERY migrations/*.sql file is bind-mounted into initdb for both
// the postgres and postgres-age services. This catches the drift where a new
// migration is added to migrations/ but not to the compose mount lists (as
// happened with 020 and 021), which would otherwise silently skip those
// migrations on a fresh `docker compose up`.
func TestEveryMigrationMountedInInitdbServices(t *testing.T) {
	repo := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(repo, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)

	files, err := filepath.Glob(filepath.Join(repo, "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no migrations/*.sql found — check test setup")
	}

	for _, service := range []string{"postgres", "postgres-age"} {
		mounted := mountedMigrations(body, service)
		if len(mounted) == 0 {
			t.Fatalf("no migration mounts parsed for service %q — parser or compose changed", service)
		}
		for _, f := range files {
			name := filepath.Base(f)
			if !mounted[name] {
				t.Errorf("migration %q is not mounted into initdb for service %q in docker-compose.yml "+
					"(add it to the service's volumes list)", name, service)
			}
		}
	}
}
