package middleware

import (
	"testing"
)

func TestIsUnauthenticatedProbe(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/health", true},
		{"GET", "/v1/health", true},
		{"GET", "/metrics", true},
		{"GET", "/ready", true},
		{"GET", "/v1/ready", true},
		{"GET", "/v1/admin/ui", false},
		{"GET", "/v1/memories/foo", false},
		{"POST", "/health", false},
		{"GET", "/v1/readyz", false},
	}
	for _, tc := range cases {
		if got := IsUnauthenticatedProbe(tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s: got %v want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
