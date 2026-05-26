// Package log provides a shared structured JSON logger for the entire PCMI codebase.
// All components should import this package and use its exported logger instead of
// the standard library log package.
package log

import (
	"log/slog"
	"os"
	"strings"
)

// lg is the global shared logger. It is initialised in init() and must not be
// replaced at runtime by individual packages.
var lg *slog.Logger

func init() {
	lg = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// SetLevel dynamically changes the log level at runtime.
func SetLevel(level slog.Level) {
	lg = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// L returns the shared logger. Callers should use L() directly rather than
// storing a reference, so that runtime level changes take effect.
func L() *slog.Logger {
	return lg
}

// Debug logs at DEBUG level.
func Debug(msg string, args ...any) {
	lg.Debug(msg, args...)
}

// Info logs at INFO level.
func Info(msg string, args ...any) {
	lg.Info(msg, args...)
}

// Warn logs at WARN level.
func Warn(msg string, args ...any) {
	lg.Warn(msg, args...)
}

// Error logs at ERROR level.
func Error(msg string, args ...any) {
	lg.Error(msg, args...)
}

// Fatal logs at ERROR level and exits with code 1.
func Fatal(msg string, args ...any) {
	lg.Error(msg, args...)
	os.Exit(1)
}

// Mask truncates a sensitive string to show only the first [prefix] characters,
// hiding the remainder with "***". This is safe for use with URLs, keys, etc.
// If the original string is shorter than [prefix] characters, it is returned as-is
// (useful in test environments where short values are acceptable).
func Mask(s string, prefix int) string {
	s = strings.TrimSpace(s)
	if len(s) <= prefix {
		return s + "***"
	}
	return s[:prefix] + "***"
}
