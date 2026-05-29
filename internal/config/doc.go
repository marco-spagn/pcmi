// Package config centralizes loading and validation of all PCMI environment variables.
// Use Load() to load the configuration and Validate() to run fail-fast validation
// at service startup. Every field has a documented default, and required variables
// are checked before any connection is opened.
package config
