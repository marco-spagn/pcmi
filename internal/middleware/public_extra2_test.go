package middleware

import (
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Wider table for IsUnauthenticatedProbe; complements public_test.go.

func TestIsUnauthenticatedProbeMethodMatrix(t *testing.T) {
	probes := []string{"/health", "/v1/health", "/metrics", "/ready", "/v1/ready"}

	// Only GET on probe paths is unauthenticated.
	for _, p := range probes {
		t.Run("GET "+p, func(t *testing.T) {
			if !IsUnauthenticatedProbe(fiber.MethodGet, p) {
				t.Fatalf("GET %s should be unauthenticated probe", p)
			}
		})
	}

	// Non-GET methods on probe paths must still require auth.
	for _, m := range []string{
		fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch,
		fiber.MethodDelete, fiber.MethodOptions, fiber.MethodHead,
	} {
		for _, p := range probes {
			t.Run(m+" "+p, func(t *testing.T) {
				if IsUnauthenticatedProbe(m, p) {
					t.Fatalf("%s %s must require authentication", m, p)
				}
			})
		}
	}

	// Arbitrary non-probe paths must require auth (even on GET).
	notProbes := []string{
		"/v1/memories",
		"/v1/memories/anything",
		"/v1/healthz",
		"/healthx",
		"/v1/readiness",
		"",
		"/",
	}
	for _, p := range notProbes {
		t.Run("GET "+p, func(t *testing.T) {
			if IsUnauthenticatedProbe(fiber.MethodGet, p) {
				t.Fatalf("GET %s must require authentication", p)
			}
		})
	}
}
