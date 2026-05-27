// Package log provides a shared structured logger for the entire PCMI codebase.
// All components should import this package and use its exported functions instead of
// the standard library log package.
//
// Defaults applied in init(): JSON format, INFO level, no source file.
// Override at runtime by calling Configure(cfg) after loading config.
//
// Env vars are read exclusively by internal/config — PCMI_LOG_FORMAT,
// PCMI_LOG_LEVEL, PCMI_LOG_SOURCE.
//
// Trace correlation: the *Context family (InfoContext, WarnContext, ErrorContext)
// automatically injects trace_id and span_id when OpenTelemetry is active.
package log

import (
	"context"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"syscall"

	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// lg is the global shared logger, protected by atomic.Pointer for safe concurrent
// reads after SetLevel / SetFormat / Configure etc. are called.
var lg atomic.Pointer[slog.Logger]

// cfg holds the current config-driven settings. Starts nil; populated by Configure().
var cfg *LogConfig

type LogConfig struct {
	Format    string // "json" (default) or "text"
	Level     string // "info" (default) | "debug" | "warn" | "error"
	AddSource bool
}

func init() {
	lg.Store(buildLogger(nil))
}

// Configure updates the logger with settings from config. Call once after
// loading config. Safe to call multiple times or before init() completes.
func Configure(format, level string, addSource bool) {
	cfg = &LogConfig{
		Format:    format,
		Level:     level,
		AddSource: addSource,
	}
	lg.Store(buildLogger(cfg))
}

// buildLogger creates the slog.Logger based on current config or defaults.
func buildLogger(c *LogConfig) *slog.Logger {
	return slog.New(buildHandler(c))
}

// buildHandler creates the slog.Handler based on current config or defaults.
func buildHandler(c *LogConfig) slog.Handler {
	out := os.Stderr

	format := "json" // default
	if c != nil && c.Format != "" {
		format = c.Format
	} else if isTTY(out.Fd()) {
		format = "text"
	}

	level := slog.LevelInfo
	if c != nil && c.Level != "" {
		if parsed := parseLevel(c.Level); parsed != 0 {
			level = parsed
		}
	}

	addSource := c != nil && c.AddSource

	h := slog.HandlerOptions{
		Level:     level,
		AddSource: addSource,
	}

	var base slog.Handler
	if format == "json" {
		base = slog.NewJSONHandler(out, &h)
	} else {
		base = slog.NewTextHandler(out, &h)
	}
	return newOTelHandler(base)
}

// isTTY returns true when fd is a terminal.
func isTTY(fd uintptr) bool {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, 0)
	return err == 0
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return 0
	}
}

// formatHandler creates a JSON or text slog.Handler writing to w.
func formatHandler(format string, w io.Writer, c *LogConfig) slog.Handler {
	level := slog.LevelInfo
	if c != nil && c.Level != "" {
		if parsed := parseLevel(c.Level); parsed != 0 {
			level = parsed
		}
	}
	addSource := false
	if c != nil {
		addSource = c.AddSource
	}
	h := slog.HandlerOptions{
		Level:     level,
		AddSource: addSource,
	}
	if format == "json" {
		return slog.NewJSONHandler(w, &h)
	}
	return slog.NewTextHandler(w, &h)
}

// SetFormat switches the logger handler format at runtime. Callers should invoke
// this once during startup if they want to override the auto-detected TTY behaviour.
func SetFormat(format string) {
	lg.Store(slog.New(newOTelHandler(formatHandler(format, os.Stderr, cfg))))
}

// SetLevel dynamically changes the log level at runtime (concurrent-safe).
func SetLevel(level slog.Level) {
	current := lg.Load()
	if current == nil {
		return
	}
	lg.Store(slog.New(newOTelHandler(levelOverride(current.Handler(), level))))
}

// levelOverride wraps a handler to enforce a new minimum level.
func levelOverride(h slog.Handler, lvl slog.Level) slog.Handler {
	return &lvlHandler{Handler: h, level: lvl}
}

type lvlHandler struct {
	slog.Handler
	level slog.Level
}

func (l *lvlHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= l.level
}

// L returns the shared logger. Callers should use L() directly rather than
// storing a reference, so that runtime level changes take effect concurrently.
func L() *slog.Logger {
	return lg.Load()
}

// Debug logs at DEBUG level.
func Debug(msg string, args ...any) {
	lg.Load().Debug(msg, args...)
}

// Info logs at INFO level.
func Info(msg string, args ...any) {
	lg.Load().Info(msg, args...)
}

// Warn logs at WARN level.
func Warn(msg string, args ...any) {
	lg.Load().Warn(msg, args...)
}

// Error logs at ERROR level.
func Error(msg string, args ...any) {
	lg.Load().Error(msg, args...)
}

// Fatal logs at ERROR level and exits with code 1.
func Fatal(msg string, args ...any) {
	lg.Load().Error(msg, args...)
	os.Exit(1)
}

// InfoContext logs at INFO level, enriching the record with trace_id/span_id
// when OpenTelemetry is active on ctx.
func InfoContext(ctx context.Context, msg string, args ...any) {
	lg.Load().Info(msg, append(traceArgs(ctx), args...)...)
}

// WarnContext logs at WARN level, enriching the record with trace_id/span_id.
func WarnContext(ctx context.Context, msg string, args ...any) {
	lg.Load().Warn(msg, append(traceArgs(ctx), args...)...)
}

// ErrorContext logs at ERROR level, enriching the record with trace_id/span_id.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	lg.Load().Error(msg, append(traceArgs(ctx), args...)...)
}

// DebugContext logs at DEBUG level, enriching the record with trace_id/span_id.
func DebugContext(ctx context.Context, msg string, args ...any) {
	lg.Load().Debug(msg, append(traceArgs(ctx), args...)...)
}

// traceArgs returns trace_id and span_id key-value pairs when ctx carries a
// sampled OTel span. Returns nil otherwise for zero-allocation append().
func traceArgs(ctx context.Context) []any {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsSampled() {
		return nil
	}
	sc := span.SpanContext()
	return []any{"trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String()}
}

// Mask truncates a sensitive string for safe logging.
//
// When the string is <= 8 characters it returns the first character followed by "***"
// (e.g. "abc" -> "a***").  For longer strings it shows the first and last 4 characters
// with "***" in between (e.g. "mysecretvalue" -> "mys****value").
//
// URL-like strings (containing "://") also attempt to redact the credentials block
// "user:pass@" if present:
//
//	"postgres://user:pass@host.db" -> "postgres://***@host.db"
func Mask(s string, prefix int) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return "***"
	}

	if strings.Contains(s, "://") {
		if masked := maskURL(s); masked != "" {
			return masked
		}
	}

	if len(s) <= 8 {
		return s[:1] + "***"
	}
	prefix = clamp(prefix, 4, 20)
	suffix := ""
	if len(s) > prefix+4 {
		suffix = s[len(s)-4:]
	}
	return s[:prefix] + "***" + suffix
}

// maskURL redacts the credentials portion of a URL-like string.
func maskURL(s string) string {
	idx := strings.Index(s, "://")
	if idx < 0 {
		return ""
	}
	before := s[:idx+3] // include "://"
	after := s[idx+3:]
	atPos := strings.Index(after, "@")
	if atPos <= 0 {
		return ""
	}
	return before + "***@" + after[atPos+1:]
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ------ OTel-aware slog.Handler wrapper ------

// otelHandler wraps another slog.Handler and injects trace_id/span_id when the
// context passed to Handle carries an active OpenTelemetry span.
type otelHandler struct {
	base slog.Handler
}

func newOTelHandler(base slog.Handler) slog.Handler {
	return &otelHandler{base: base}
}

func (h *otelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *otelHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsSampled() {
		sc := span.SpanContext()
		r = r.Clone()
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()))
	}
	return h.base.Handle(ctx, r)
}

func (h *otelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &otelHandler{base: h.base.WithAttrs(attrs)}
}

func (h *otelHandler) WithGroup(name string) slog.Handler {
	return &otelHandler{base: h.base.WithGroup(name)}
}
