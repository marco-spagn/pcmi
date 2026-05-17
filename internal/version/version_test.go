package version

import (
	"strings"
	"testing"
)

func TestTagHasVPrefix(t *testing.T) {
	if !strings.HasPrefix(Tag, "v") {
		t.Fatalf("Tag must start with 'v', got %q", Tag)
	}
}

func TestSemverNoVPrefix(t *testing.T) {
	if strings.HasPrefix(Semver, "v") {
		t.Fatalf("Semver must not start with 'v', got %q", Semver)
	}
}

func TestTagSemverConsistent(t *testing.T) {
	// Tag must be "v" + Semver
	want := "v" + Semver
	if Tag != want {
		t.Fatalf("Tag=%q does not match 'v'+Semver=%q", Tag, want)
	}
}

func TestSemverFormat(t *testing.T) {
	parts := strings.Split(Semver, ".")
	if len(parts) != 3 {
		t.Fatalf("Semver must have 3 components (MAJOR.MINOR.PATCH), got %q", Semver)
	}
	for _, p := range parts {
		if len(p) == 0 {
			t.Fatalf("Semver component must not be empty in %q", Semver)
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				t.Fatalf("Semver component %q must contain only digits in %q", p, Semver)
			}
		}
	}
}
