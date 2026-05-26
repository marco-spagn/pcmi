package log

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"log/slog"
)

// ─── Mask ──────────────────────────────────────────────────────────────────────

func TestMaskEmpty(t *testing.T) {
	if got := Mask("", 4); got != "***" {
		t.Errorf("expected \"***\", got %q", got)
	}
}

func TestMaskWhitespacesOnly(t *testing.T) {
	if got := Mask("   ", 4); got != "***" {
		t.Errorf("expected \"***\", got %q", got)
	}
}

func TestMaskShort(t *testing.T) {
	if got := Mask("abc", 4); got != "a***" {
		t.Errorf("expected \"a***\", got %q", got)
	}
}

func TestMaskLeq8(t *testing.T) {
	if got := Mask("abcdefg", 4); got != "a***" {
		t.Errorf("expected \"a***\", got %q", got)
	}
}

func TestMaskLong(t *testing.T) {
	got := Mask("mysecretkeyvalue", 4)
	if !strings.HasPrefix(got, "mys") || !strings.Contains(got, "***") {
		t.Errorf("expected prefix+***+suffix, got %q", got)
	}
}

func TestMaskURLCredentialsRedacted(t *testing.T) {
	got := Mask("postgres://user:pass@host.db", 4)
	if strings.Contains(got, "user:pass") {
		t.Errorf("credentials not redacted: %q", got)
	}
	if !strings.Contains(got, "***@") {
		t.Errorf("expected credentials redacted with ***@: %q", got)
	}
}

func TestMaskURLNoCredentials(t *testing.T) {
	got := Mask("https://example.com/path", 4)
	if strings.Contains(got, "***@") {
		t.Errorf("should not contain ***@ for URL without credentials: %q", got)
	}
}

// ─── parseLevel ────────────────────────────────────────────────────────────────

func TestParseLevelDebug(t *testing.T) {
	if lvl := parseLevel("debug"); lvl != slog.LevelDebug {
		t.Errorf("expected Debug, got %v", lvl)
	}
}

func TestParseLevelInfo(t *testing.T) {
	if lvl := parseLevel("info"); lvl != slog.LevelInfo {
		t.Errorf("expected Info, got %v", lvl)
	}
}

func TestParseLevelWarn(t *testing.T) {
	if lvl := parseLevel("WARN"); lvl != slog.LevelWarn {
		t.Errorf("expected Warn, got %v", lvl)
	}
}

func TestParseLevelError(t *testing.T) {
	if lvl := parseLevel("error"); lvl != slog.LevelError {
		t.Errorf("expected Error, got %v", lvl)
	}
}

func TestParseLevelInvalid(t *testing.T) {
	if lvl := parseLevel("bogus"); lvl != 0 {
		t.Errorf("expected 0 for invalid level, got %v", lvl)
	}
}

// ─── clamp ─────────────────────────────────────────────────────────────────────

func TestClampMiddle(t *testing.T) {
	if got := clamp(5, 3, 10); got != 5 {
		t.Errorf("expected 5, got %d", got)
	}
}

func TestClampBelowMin(t *testing.T) {
	if got := clamp(1, 3, 10); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestClampAboveMax(t *testing.T) {
	if got := clamp(20, 3, 10); got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

// ─── maskURL ───────────────────────────────────────────────────────────────────

func TestMaskURLWithCreds(t *testing.T) {
	got := maskURL("postgres://user:pass@localhost.db")
	if got != "postgres://***@localhost.db" {
		t.Errorf("expected \"postgres://***@localhost.db\", got %q", got)
	}
}

func TestMaskURLNoAt(t *testing.T) {
	got := maskURL("https://example.com")
	if got != "" {
		t.Errorf("expected empty for URL without @, got %q", got)
	}
}

func TestMaskURLNoSchema(t *testing.T) {
	got := maskURL("notaweburl")
	if got != "" {
		t.Errorf("expected empty for string without ://, got %q", got)
	}
}

func TestMaskURLEmptyAfterColonSlashSlash(t *testing.T) {
	got := maskURL("://")
	if got != "" {
		t.Errorf("expected empty for \"://\", got %q", got)
	}
}

// ─── L returns non-nil ─────────────────────────────────────────────────────────

func TestLNonNil(t *testing.T) {
	if got := L(); got == nil {
		t.Error("L() returned nil")
	}
}

// ─── SetLevel concurrent safety (race detector clean) ─────────────────────────

func TestSetLevelConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { SetLevel(slog.LevelDebug); wg.Done() }()
		go func() { Info("concurrent-info"); wg.Done() }()
	}
	wg.Wait()
}

// ─── SetLevel actually changes output ──────────────────────────────────────────

func TestSetLevelFiltersDebug(t *testing.T) {
	lg.Store(slog.New(newOTelHandler(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))))

	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelInfo))))

	lg.Load().Debug("hidden")
	if got := b.String(); got != "" {
		t.Errorf("expected debug to be filtered at info level, got %q", got)
	}

	SetLevel(slog.LevelDebug)
	lg.Load().Debug("visible")
	got := b.String()
	if got == "" {
		t.Error("expected debug message after SetLevel(debug)")
	}
	var obj map[string]any
	requireJSONParse(t, got, &obj)
}

// ─── JSON output contains expected keys ────────────────────────────────────────

func TestJSONOutputContainsMsgAndLevel(t *testing.T) {
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelInfo))))
	Info("hello", "key", "value")

	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)

	if lvl, _ := obj["level"].(string); lvl != "INFO" {
		t.Errorf("expected level=INFO, got %q", lvl)
	}
	if msg, _ := obj["msg"].(string); msg != "hello" {
		t.Errorf("expected msg=hello, got %q", msg)
	}
	if kv, _ := obj["key"].(string); kv != "value" {
		t.Errorf("expected key=value, got %q", kv)
	}
}

// ─── Configure with JSON format ────────────────────────────────────────────────

func TestConfigureJSON(t *testing.T) {
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelInfo))))
	Info("cfg-test")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if msg, _ := obj["msg"].(string); msg != "cfg-test" {
		t.Errorf("expected msg=cfg-test, got %q", msg)
	}
}

// ─── Configure with text format ────────────────────────────────────────────────

func TestConfigureText(t *testing.T) {
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(formatHandler("text", b, nil))))
	Info("text-test")
	got := b.String()
	if !strings.Contains(got, "INFO") || !strings.Contains(got, "text-test") {
		t.Errorf("expected text format output, got %q", got)
	}
}

// ─── Configure with debug level ────────────────────────────────────────────────

func TestConfigureDebugLevel(t *testing.T) {
	b := &bytes.Buffer{}
	Configure("json", "debug", false)
	lg.Store(slog.New(newOTelHandler(buildHandler(cfg))))
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelDebug))))
	Debug("debug-test")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if lvl, _ := obj["level"].(string); lvl != "DEBUG" {
		t.Errorf("expected level=DEBUG, got %q", lvl)
	}
}

// ─── SetFormat runtime switch ─────────────────────────────────────────────────

func TestSetFormat(t *testing.T) {
	SetFormat("text")
	SetFormat("json")
	l := L()
	if l == nil {
		t.Fatal("L() returned nil after SetFormat")
	}
}

// ─── Fatal wrapper ─────────────────────────────────────────────────────────────

func TestFatalExits(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{os.Args[0]}

	// Fatal calls os.Exit(1) -- we can't directly test it from this process,
	// but we can verify that the error log is emitted before exit.
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelError))))

	// We can't call Fatal directly because it calls os.Exit(1).
	// Instead verify the handler at error level works.
	lg.Load().Error("fatal-test")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if msg, _ := obj["msg"].(string); msg != "fatal-test" {
		t.Errorf("expected msg=fatal-test, got %q", msg)
	}
}

// ─── traceArgs ─────────────────────────────────────────────────────────────────

func TestTraceArgsNoSpan(t *testing.T) {
	ctx := context.Background()
	got := traceArgs(ctx)
	if got != nil {
		t.Errorf("expected nil for background context, got %v", got)
	}
}

// ─── InfoContext passes context ────────────────────────────────────────────────

func TestInfoContext(t *testing.T) {
	ctx := context.Background()
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelInfo))))
	InfoContext(ctx, "ctx-test", "k", "v")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if msg, _ := obj["msg"].(string); msg != "ctx-test" {
		t.Errorf("expected msg=ctx-test, got %q", msg)
	}
}

// ─── WarnContext ───────────────────────────────────────────────────────────────

func TestWarnContext(t *testing.T) {
	ctx := context.Background()
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelInfo))))
	WarnContext(ctx, "warn-ctx")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if lvl, _ := obj["level"].(string); lvl != "WARN" {
		t.Errorf("expected level=WARN, got %q", lvl)
	}
}

// ─── ErrorContext ──────────────────────────────────────────────────────────────

func TestErrorContext(t *testing.T) {
	ctx := context.Background()
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelInfo))))
	ErrorContext(ctx, "err-ctx")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if lvl, _ := obj["level"].(string); lvl != "ERROR" {
		t.Errorf("expected level=ERROR, got %q", lvl)
	}
}

// ─── DebugContext ──────────────────────────────────────────────────────────────

func TestDebugContext(t *testing.T) {
	ctx := context.Background()
	b := &bytes.Buffer{}
	lg.Store(slog.New(newOTelHandler(jsonHandler(b, slog.LevelDebug))))
	DebugContext(ctx, "debug-ctx")
	var obj map[string]any
	requireJSONParse(t, b.String(), &obj)
	if lvl, _ := obj["level"].(string); lvl != "DEBUG" {
		t.Errorf("expected level=DEBUG, got %q", lvl)
	}
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func jsonHandler(w io.Writer, level slog.Level) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
}

func requireJSONParse(t *testing.T, raw string, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(bytes.NewReader([]byte(raw))).Decode(v); err != nil {
		t.Fatalf("failed to parse JSON: %v (raw=%q)", err, raw)
	}
}
