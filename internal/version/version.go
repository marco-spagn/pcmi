// Package version holds the single release version string for HTTP health, gRPC, worker, and CI smoke.
package version

const (
	// Tag is the public API version (e.g. "v1.24.0").
	Tag = "v1.24.0"
	// Semver is OpenAPI info.version without the "v" prefix.
	Semver = "1.24.0"
)
