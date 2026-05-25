package middleware

import "testing"

func TestIsUnauthenticatedProbePOSTBlocked(t *testing.T) {
	blocked := []struct{ method, path string }{
		{"POST", "/health"},
		{"POST", "/metrics"},
		{"DELETE", "/v1/health"},
		{"PUT", "/ready"},
	}
	for _, tc := range blocked {
		if IsUnauthenticatedProbe(tc.method, tc.path) {
			t.Errorf("%s %s should NOT be unauthenticated probe", tc.method, tc.path)
		}
	}
}

func TestIsUnauthenticatedProbeAllProbes(t *testing.T) {
	probes := []string{"/health", "/v1/health", "/metrics", "/ready", "/v1/ready", "/v1/admin/ui"}
	for _, p := range probes {
		if !IsUnauthenticatedProbe("GET", p) {
			t.Errorf("GET %s should be unauthenticated probe", p)
		}
	}
}

func TestIsUnauthenticatedProbeOtherPaths(t *testing.T) {
	others := []string{
		"/v1/memories/foo",
		"/v1/audit",
		"/v1/distilled",
		"/api/health",
		"/healthz",
	}
	for _, p := range others {
		if IsUnauthenticatedProbe("GET", p) {
			t.Errorf("GET %s should NOT be unauthenticated probe", p)
		}
	}
}
