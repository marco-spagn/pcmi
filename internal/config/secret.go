package config

import (
	"log"
	"os"
	"strings"
)

// resolveSecret returns the value of a sensitive setting while letting operators
// keep the raw secret OUT of the process environment — the finding every pentest
// flags. Indirection is resolved in this order:
//
//	<NAME>_FILE=/run/secrets/x   read the secret from a file (Docker secrets,
//	                             Kubernetes mounted secrets, a Vault Agent sidecar
//	                             that writes to a file). Takes precedence.
//	<NAME>=file:/run/secrets/x   same, expressed inline as a scheme
//	<NAME>=env:OTHER_VAR         indirect to another environment variable
//	<NAME>=<literal>             used verbatim (backward compatible)
//
// File contents are trimmed of surrounding whitespace/newlines so a secret file
// ending in a newline behaves like the raw value. When a file is referenced but
// cannot be read, resolveSecret logs a warning and returns "" — Validate() then
// reports the missing-secret error, rather than the process silently running
// with an empty or wrong secret.
func resolveSecret(name string) string {
	if fileVar := strings.TrimSpace(os.Getenv(name + "_FILE")); fileVar != "" {
		return readSecretFile(fileVar, name+"_FILE")
	}

	raw := strings.TrimSpace(os.Getenv(name))
	switch {
	case strings.HasPrefix(raw, "file:"):
		return readSecretFile(strings.TrimPrefix(raw, "file:"), name)
	case strings.HasPrefix(raw, "env:"):
		indirect := strings.TrimSpace(strings.TrimPrefix(raw, "env:"))
		if indirect == "" {
			log.Printf("WARNING: %s uses env: indirection but names no variable", name)
			return ""
		}
		return strings.TrimSpace(os.Getenv(indirect))
	default:
		return raw
	}
}

// readSecretFile reads a secret from disk. The path is operator-supplied
// configuration by design, so the file-inclusion lint is intentionally waived.
func readSecretFile(path, source string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator-provided secret file path by design
	if err != nil {
		log.Printf("WARNING: %s points to %q but it could not be read: %v — treating secret as unset", source, path, err)
		return ""
	}
	return strings.TrimSpace(string(b))
}
