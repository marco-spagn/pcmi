package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PCMI-016: AI framework integration examples under examples/{langchain,llamaindex,autogen,crewai}.
var aiFrameworkExampleDirs = []string{
	"langchain",
	"llamaindex",
	"autogen",
	"crewai",
}

// TestAIFrameworkExamplesLayout guards the PCMI-016 example tree (no Python runtime required).
func TestAIFrameworkExamplesLayout(t *testing.T) {
	repo := repoRoot(t)
	examples := filepath.Join(repo, "examples")

	shared := filepath.Join(examples, "pcmi_http.py")
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("missing shared helper %s: %v", shared, err)
	}
	body := string(readFile(t, shared))
	for _, needle := range []string{"/v1/memories", "/v1/retrieve", "/v1/sessions"} {
		if !strings.Contains(body, needle) {
			t.Errorf("pcmi_http.py missing %q", needle)
		}
	}

	readme := string(readFile(t, filepath.Join(examples, "README.md")))
	for _, name := range aiFrameworkExampleDirs {
		if !strings.Contains(readme, name+"/") {
			t.Errorf("examples/README.md should link to %s/", name)
		}
	}

	for _, dir := range aiFrameworkExampleDirs {
		base := filepath.Join(examples, dir)
		for _, file := range []string{"README.md", "requirements.txt", "smoke_test.py", "main.py"} {
			path := filepath.Join(base, file)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
		text := strings.ToLower(string(readFile(t, filepath.Join(base, "README.md"))))
		if !strings.Contains(text, "pcmi_api_key") {
			t.Errorf("%s README should document PCMI_API_KEY", dir)
		}
	}
}
