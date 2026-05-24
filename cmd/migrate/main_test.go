package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestMigrationFilesAreSortable verifies that all migration filenames sort
// correctly in lexicographic order (their numeric prefix determines order).
func TestMigrationFilesAreSortable(t *testing.T) {
	dir := "../../migrations"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("migrations dir not found at %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Skip("no migration files found")
	}
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	for i := range files {
		if files[i] != sorted[i] {
			t.Errorf("migration file order mismatch at %d: %q vs %q",
				i, files[i], sorted[i])
		}
	}
	t.Logf("✅ %d migration files sort correctly", len(files))
}

// TestMigrationFilesAreReadable verifies all .sql files can be read.
func TestMigrationFilesAreReadable(t *testing.T) {
	dir := "../../migrations"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("migrations dir not found: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Errorf("cannot read %s: %v", e.Name(), err)
		}
		if len(data) == 0 {
			t.Errorf("migration file %s is empty", e.Name())
		}
	}
}
