package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProtoMemoryServiceProtoExists ensures the gRPC contract source is present
// under proto/pcmi/v1/ (generated *.pb.go live in internal/grpc/pcmiv1).
func TestProtoMemoryServiceProtoExists(t *testing.T) {
	repo := repoRoot(t)
	path := filepath.Join(repo, "proto", "pcmi", "v1", "memory.proto")
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		t.Fatalf("missing proto/pcmi/v1/memory.proto")
	}
	body := string(readFile(t, path))
	for _, needle := range []string{"syntax = \"proto3\";", "package pcmi.v1;", "service MemoryService", "rpc Store("} {
		if !strings.Contains(body, needle) {
			t.Fatalf("memory.proto missing %q", needle)
		}
	}
}
