// pcmi is the developer CLI for a running PCMI API: store and retrieve
// memories, tail live events, seed demo data, read usage, and work with the
// tamper-evident audit chain (including offline verification of exports).
//
//	export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=...
//	pcmi seed
//	pcmi retrieve --query "what broke after the deploy?"
//	pcmi tail
//
// See docs/CLI.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/marco-spagn/pcmi/internal/auditchain"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/version"
)

const usageText = `pcmi — developer CLI for PCMI

Usage:
  pcmi [global flags] <command> [flags] [args]

Commands:
  store <path> [content|-]    Store a memory (content from stdin when "-" or omitted)
  get <path>                  Read the current (or --version / --as-of) memory at path
  retrieve                    Hybrid retrieve (--query, --prefix, --limit, --as-of)
  tail                        Stream live events (SSE) until Ctrl-C
  seed                        Store a demo corpus (or --file memories.jsonl)
  usage                       LLM / embedding token usage and estimated cost
  erase <prefix>              GDPR erasure under a path prefix (admin; --dry-run first)
  audit verify                Verify the tenant's audit hash chain on the server
  audit export                Download a sealed JSONL audit export (admin)
  audit verify-export <file>  Verify an export offline (no server needed)
  version                     Print CLI and server versions

Global flags:
  --url URL        API base URL (default $PCMI_BASE_URL or http://localhost:8000)
  --api-key KEY    API key (default $PCMI_API_KEY; PCMI_API_KEY_FILE supported)
  --json           Print raw JSON responses
  --timeout DUR    Per-request timeout (default 30s)
`

type app struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	cfg    *config.Config

	client  *apiClient
	jsonOut bool
}

// errUsage marks a command-line mistake (exit code 2).
var errUsage = errors.New("usage")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a := &app{ctx: ctx, stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, cfg: config.Load()}
	os.Exit(a.run(os.Args[1:]))
}

func (a *app) run(args []string) int {
	global := flag.NewFlagSet("pcmi", flag.ContinueOnError)
	global.SetOutput(a.stderr)
	global.Usage = func() { a.errf("%s", usageText) }
	baseURL := global.String("url", firstNonEmpty(a.cfg.PCMIBaseURL, "http://localhost:8000"), "API base URL")
	apiKey := global.String("api-key", a.cfg.PCMIAPIKey, "API key")
	global.BoolVar(&a.jsonOut, "json", false, "print raw JSON")
	timeout := global.Duration("timeout", 30*time.Second, "per-request timeout")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := global.Args()
	if len(rest) == 0 {
		a.errf("%s", usageText)
		return 2
	}
	a.client = newAPIClient(*baseURL, *apiKey, *timeout)

	cmd, cmdArgs := rest[0], rest[1:]
	var err error
	switch cmd {
	case "store":
		err = a.cmdStore(cmdArgs)
	case "get":
		err = a.cmdGet(cmdArgs)
	case "retrieve", "search":
		err = a.cmdRetrieve(cmdArgs)
	case "tail":
		err = a.cmdTail(cmdArgs)
	case "seed":
		err = a.cmdSeed(cmdArgs)
	case "usage":
		err = a.cmdUsage(cmdArgs)
	case "erase":
		err = a.cmdErase(cmdArgs)
	case "audit":
		err = a.cmdAudit(cmdArgs)
	case "version":
		err = a.cmdVersion(cmdArgs)
	case "help", "-h", "--help":
		a.outf("%s", usageText)
		return 0
	default:
		a.errf("pcmi: unknown command %q\n\n%s", cmd, usageText)
		return 2
	}
	return a.exitCode(err)
}

// errInvalid marks a completed check whose verdict is negative (exit code 1,
// no extra error line — the command already printed the verdict).
var errInvalid = errors.New("invalid")

func (a *app) exitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errInvalid):
		return 1
	case errors.Is(err, errUsage), errors.Is(err, flag.ErrHelp):
		if !errors.Is(err, flag.ErrHelp) && err.Error() != errUsage.Error() {
			a.errf("pcmi: %v\n", err)
		}
		return 2
	default:
		a.errf("pcmi: %v\n", err)
		return 1
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func usageErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errUsage}, args...)...)
}

// newFlags returns a subcommand FlagSet that reports errors as usage errors.
func (a *app) newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	return fs
}

// parseInterspersed lets flags follow positional arguments
// (`pcmi store root.a "text" --tag x`), which the flag package does not.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, fmt.Errorf("%w: %v", errUsage, err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// multiFlag collects a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// outf / outln / errf write CLI output; a failed terminal write has no
// better recovery than dropping the text, so the error is discarded.
func (a *app) outf(format string, args ...any) { _, _ = fmt.Fprintf(a.stdout, format, args...) }
func (a *app) outln(args ...any)               { _, _ = fmt.Fprintln(a.stdout, args...) }
func (a *app) errf(format string, args ...any) { _, _ = fmt.Fprintf(a.stderr, format, args...) }

func (a *app) printJSON(v any) error {
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// ─── store / get / retrieve ────────────────────────────────────────────────

func (a *app) cmdStore(args []string) error {
	fs := a.newFlags("store")
	var tags, meta multiFlag
	fs.Var(&tags, "tag", "tag (repeatable)")
	fs.Var(&meta, "meta", "metadata key=value (repeatable; value parsed as JSON when possible)")
	importance := fs.Float64("importance", -1, "importance 0..1 (default server 0.5)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 || len(pos) > 2 {
		return usageErr("store <path> [content|-]")
	}
	content := ""
	if len(pos) == 2 && pos[1] != "-" {
		content = pos[1]
	} else {
		b, err := io.ReadAll(a.stdin)
		if err != nil {
			return err
		}
		content = strings.TrimRight(string(b), "\n")
	}
	if strings.TrimSpace(content) == "" {
		return usageErr("content is empty")
	}
	metadata := map[string]any{}
	for _, kv := range meta {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return usageErr("--meta expects key=value, got %q", kv)
		}
		var parsed any
		if json.Unmarshal([]byte(v), &parsed) == nil {
			metadata[k] = parsed
		} else {
			metadata[k] = v
		}
	}
	body := map[string]any{"path": pos[0], "content": content, "metadata": metadata}
	if len(tags) > 0 {
		body["tags"] = []string(tags)
	}
	if *importance >= 0 {
		body["importance"] = *importance
	}
	var out map[string]any
	if err := a.client.json(a.ctx, "POST", "/v1/memories", body, &out); err != nil {
		return err
	}
	if a.jsonOut {
		return a.printJSON(out)
	}
	a.outf("stored %s  id=%v version=%v status=%v\n", pos[0], out["id"], out["version"], out["status"])
	return nil
}

type memoryEntry struct {
	ID             int64    `json:"id"`
	Path           string   `json:"path"`
	Content        string   `json:"content"`
	Version        int      `json:"version"`
	Tags           []string `json:"tags"`
	RelevanceScore float64  `json:"relevance_score"`
	ValidFrom      string   `json:"valid_from"`
	ValidTo        *string  `json:"valid_to"`
}

func (a *app) cmdGet(args []string) error {
	fs := a.newFlags("get")
	ver := fs.Int("version", 0, "exact version")
	asOf := fs.String("as-of", "", "RFC3339 point in time (time travel)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("get <path> [--version N | --as-of RFC3339]")
	}
	q := url.Values{}
	if *ver > 0 {
		q.Set("version", strconv.Itoa(*ver))
	}
	if *asOf != "" {
		q.Set("as_of", *asOf)
	}
	path := "/v1/memories/" + pos[0]
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	data, _, err := a.client.raw(a.ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	if a.jsonOut {
		_, err := a.stdout.Write(append(data, '\n'))
		return err
	}
	var e memoryEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}
	a.outf("%s  v%d  valid_from=%s\n%s\n", e.Path, e.Version, e.ValidFrom, e.Content)
	return nil
}

func (a *app) cmdRetrieve(args []string) error {
	fs := a.newFlags("retrieve")
	query := fs.String("query", "", "semantic + lexical query")
	fs.StringVar(query, "q", "", "shorthand for --query")
	prefix := fs.String("prefix", "", "ltree path prefix (default: whole tenant)")
	limit := fs.Int("limit", 10, "max results (1-200)")
	asOf := fs.String("as-of", "", "RFC3339 point in time (time travel)")
	noRerank := fs.Bool("no-rerank", false, "opt out of server-side LLM reranking")
	var tags multiFlag
	fs.Var(&tags, "tag", "tag filter (repeatable)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if *query == "" && len(pos) > 0 {
		*query = strings.Join(pos, " ")
	}
	body := map[string]any{"path_prefix": *prefix, "query": *query, "limit": *limit}
	if *asOf != "" {
		body["as_of"] = *asOf
	}
	if len(tags) > 0 {
		body["tags"] = []string(tags)
	}
	if *noRerank {
		body["rerank"] = false
	}
	data, _, err := a.client.raw(a.ctx, "POST", "/v1/retrieve", body)
	if err != nil {
		return err
	}
	if a.jsonOut {
		_, err := a.stdout.Write(append(data, '\n'))
		return err
	}
	var resp struct {
		Entries  []memoryEntry `json:"entries"`
		Reranked bool          `json:"reranked"`
		HasMore  bool          `json:"has_more"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	if len(resp.Entries) == 0 {
		a.outln("no memories matched")
		return nil
	}
	tw := tabwriter.NewWriter(a.stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "#\tSCORE\tPATH\tV\tCONTENT")
	for i, e := range resp.Entries {
		score := "-"
		if e.RelevanceScore != 0 {
			score = fmt.Sprintf("%.3f", e.RelevanceScore)
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n", i+1, score, e.Path, e.Version, snippet(e.Content, 80))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	var notes []string
	if resp.Reranked {
		notes = append(notes, "reranked by LLM")
	}
	if resp.HasMore {
		notes = append(notes, "more results available (--json for next_cursor)")
	}
	if len(notes) > 0 {
		a.outf("(%s)\n", strings.Join(notes, "; "))
	}
	return nil
}

// ─── tail ──────────────────────────────────────────────────────────────────

func (a *app) cmdTail(args []string) error {
	fs := a.newFlags("tail")
	types := fs.String("types", "", "comma-separated event types (default: all)")
	count := fs.Int("n", 0, "exit after N events (0 = until Ctrl-C)")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	path := "/v1/events"
	if *types != "" {
		path += "?" + url.Values{"types": {*types}}.Encode()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	seen := 0
	if !a.jsonOut {
		a.errf("tailing %s%s (Ctrl-C to stop)\n", a.client.base, path)
	}
	err := a.client.stream(ctx, path, func(ev sseEvent) error {
		if a.jsonOut {
			a.outln(ev.Data)
		} else {
			var msg struct {
				Type    string         `json:"type"`
				Payload map[string]any `json:"payload"`
			}
			name := ev.Name
			summary := ev.Data
			if json.Unmarshal([]byte(ev.Data), &msg) == nil {
				if msg.Type != "" {
					name = msg.Type
				}
				if p, ok := msg.Payload["path"]; ok {
					summary = fmt.Sprintf("path=%v", p)
				}
			}
			a.outf("%s  %-22s %s\n", time.Now().Format("15:04:05"), name, summary)
		}
		seen++
		if *count > 0 && seen >= *count {
			cancel()
		}
		return nil
	})
	if err != nil && ctx.Err() != nil {
		return nil // stopped by Ctrl-C or -n
	}
	return err
}

// ─── seed ──────────────────────────────────────────────────────────────────

func (a *app) cmdSeed(args []string) error {
	fs := a.newFlags("seed")
	file := fs.String("file", "", "JSONL file of memories ({path, content, metadata, tags, importance})")
	prefix := fs.String("prefix", "root.demo", "path prefix for the built-in demo corpus")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	var mems []seedMemory
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if mems, err = readSeedFile(f); err != nil {
			return fmt.Errorf("%s: %w", *file, err)
		}
	} else {
		p := strings.Trim(strings.TrimSpace(*prefix), ".")
		for _, m := range demoCorpus {
			m.Path = p + "." + m.Path
			mems = append(mems, m)
		}
	}
	stored := 0
	for _, m := range mems {
		body := map[string]any{"path": m.Path, "content": m.Content, "metadata": m.Metadata}
		if m.Metadata == nil {
			body["metadata"] = map[string]any{}
		}
		if len(m.Tags) > 0 {
			body["tags"] = m.Tags
		}
		if m.Importance != nil {
			body["importance"] = *m.Importance
		}
		if err := a.client.json(a.ctx, "POST", "/v1/memories", body, nil); err != nil {
			return fmt.Errorf("store %s: %w", m.Path, err)
		}
		stored++
	}
	a.outf("stored %d memories\n", stored)
	if *file == "" && !a.jsonOut {
		p := strings.Trim(strings.TrimSpace(*prefix), ".")
		a.outf(`
Try:
  pcmi retrieve --prefix %[1]s --query "what went wrong after the deploy?"
  pcmi get %[1]s.product.pricing.plan --version 1     # time travel: the first price
  pcmi tail                                             # then store something in another shell
`, p)
	}
	return nil
}

// ─── usage ─────────────────────────────────────────────────────────────────

func (a *app) cmdUsage(args []string) error {
	fs := a.newFlags("usage")
	from := fs.String("from", "", "first UTC day YYYY-MM-DD (default: 30 days ago)")
	to := fs.String("to", "", "last UTC day YYYY-MM-DD (default: today)")
	groupBy := fs.String("group-by", "", "day,operation,model or none (default operation,model)")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	q := url.Values{}
	for k, v := range map[string]string{"from": *from, "to": *to, "group_by": *groupBy} {
		if v != "" {
			q.Set(k, v)
		}
	}
	path := "/v1/stats/usage"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	data, _, err := a.client.raw(a.ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	if a.jsonOut {
		_, err := a.stdout.Write(append(data, '\n'))
		return err
	}
	type line struct {
		Day          string   `json:"day"`
		Operation    string   `json:"operation"`
		Provider     string   `json:"provider"`
		Model        string   `json:"model"`
		Requests     int64    `json:"requests"`
		InputTokens  int64    `json:"input_tokens"`
		OutputTokens int64    `json:"output_tokens"`
		Cost         *float64 `json:"estimated_cost_usd"`
	}
	var rep struct {
		From           string   `json:"from"`
		To             string   `json:"to"`
		Rows           []line   `json:"rows"`
		Totals         line     `json:"totals"`
		UnpricedModels []string `json:"unpriced_models"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return err
	}
	cost := func(c *float64) string {
		if c == nil {
			return "n/a"
		}
		return fmt.Sprintf("$%.4f", *c)
	}
	a.outf("usage %s → %s\n", rep.From, rep.To)
	withDay := false
	for _, r := range rep.Rows {
		withDay = withDay || r.Day != ""
	}
	day := func(d string) string {
		if withDay {
			return d + "\t"
		}
		return ""
	}
	tw := tabwriter.NewWriter(a.stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, day("DAY")+"OPERATION\tMODEL\tREQUESTS\tIN TOKENS\tOUT TOKENS\tCOST")
	for _, r := range rep.Rows {
		_, _ = fmt.Fprintf(tw, "%s%s\t%s\t%d\t%d\t%d\t%s\n", day(r.Day), r.Operation, r.Model, r.Requests, r.InputTokens, r.OutputTokens, cost(r.Cost))
	}
	t := rep.Totals
	_, _ = fmt.Fprintf(tw, "%sTOTAL\t\t%d\t%d\t%d\t%s\n", day(""), t.Requests, t.InputTokens, t.OutputTokens, cost(t.Cost))
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(rep.UnpricedModels) > 0 {
		a.outf("(no LLM_PRICING entry for: %s)\n", strings.Join(rep.UnpricedModels, ", "))
	}
	return nil
}

// ─── erase ─────────────────────────────────────────────────────────────────

func (a *app) cmdErase(args []string) error {
	fs := a.newFlags("erase")
	dry := fs.Bool("dry-run", false, "report exact counts without deleting")
	reason := fs.String("reason", "", "reason recorded in the gdpr_erase audit row")
	yes := fs.Bool("yes", false, "confirm the irreversible erasure")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("erase <prefix> [--dry-run | --yes] [--reason TEXT]")
	}
	if !*dry && !*yes {
		return usageErr("erasure is irreversible: run with --dry-run first, then again with --yes")
	}
	var out map[string]any
	body := map[string]any{"path_prefix": pos[0], "dry_run": *dry, "reason": *reason}
	if err := a.client.json(a.ctx, "POST", "/v1/memories/erase", body, &out); err != nil {
		return err
	}
	if a.jsonOut {
		return a.printJSON(out)
	}
	verb := "erased"
	if *dry {
		verb = "would erase"
	}
	a.outf("%s under %s:\n", verb, pos[0])
	if counts, ok := out["counts"].(map[string]any); ok {
		for _, k := range []string{"memory_versions", "paths", "links", "distilled", "link_proposals", "entity_snapshots", "alias_proposals", "consolidation_runs"} {
			a.outf("  %-19s %v\n", k, counts[k])
		}
	}
	if ge, ok := out["graph_error"].(string); ok && ge != "" {
		a.outf("  graph cleanup FAILED: %s\n", ge)
	}
	return nil
}

// ─── audit ─────────────────────────────────────────────────────────────────

func (a *app) cmdAudit(args []string) error {
	if len(args) == 0 {
		return usageErr("audit verify | audit export [-o FILE] | audit verify-export FILE")
	}
	switch args[0] {
	case "verify":
		return a.auditVerify(args[1:])
	case "export":
		return a.auditExport(args[1:])
	case "verify-export":
		return a.auditVerifyExport(args[1:])
	default:
		return usageErr("unknown audit subcommand %q", args[0])
	}
}

func (a *app) auditVerify(args []string) error {
	if _, err := parseInterspersed(a.newFlags("audit verify"), args); err != nil {
		return err
	}
	var res struct {
		Valid      bool              `json:"valid"`
		Checked    int64             `json:"checked"`
		HeadSeq    int64             `json:"head_seq"`
		HeadHash   string            `json:"head_hash"`
		FirstBreak *auditchain.Break `json:"first_break"`
	}
	data, _, err := a.client.raw(a.ctx, "GET", "/v1/audit/verify", nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return err
	}
	if a.jsonOut {
		_, _ = a.stdout.Write(append(data, '\n'))
	} else if res.Valid {
		a.outf("✓ audit chain intact: %d rows, head seq %d, head %s\n", res.Checked, res.HeadSeq, res.HeadHash)
	} else {
		a.outf("✗ audit chain BROKEN at seq %d (%s) after checking %d rows\n",
			res.FirstBreak.ChainSeq, res.FirstBreak.Reason, res.Checked)
	}
	if !res.Valid {
		return errInvalid
	}
	return nil
}

func (a *app) auditExport(args []string) error {
	fs := a.newFlags("audit export")
	out := fs.String("o", "", "output file (default stdout)")
	fromSeq := fs.Int64("from-seq", 0, "first chain_seq")
	toSeq := fs.Int64("to-seq", 0, "last chain_seq (0 = head)")
	limit := fs.Int("limit", 0, "max rows (server default 10000, max 50000)")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	q := url.Values{}
	if *fromSeq > 0 {
		q.Set("from_seq", strconv.FormatInt(*fromSeq, 10))
	}
	if *toSeq > 0 {
		q.Set("to_seq", strconv.FormatInt(*toSeq, 10))
	}
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	path := "/v1/audit/export"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	data, hdr, err := a.client.raw(a.ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	w := a.stdout
	if *out != "" {
		if err := os.WriteFile(*out, data, 0o600); err != nil {
			return err
		}
		a.errf("wrote %s (%d bytes, head %s)\n", *out, len(data), hdr.Get("X-PCMI-Audit-Head-Hash"))
	} else if _, err := w.Write(data); err != nil {
		return err
	}
	if hdr.Get("X-PCMI-Audit-Complete") == "false" {
		a.errf("note: export is partial — resume with --from-seq <trailer.next_from_seq>\n")
	}
	return nil
}

func (a *app) auditVerifyExport(args []string) error {
	fs := a.newFlags("audit verify-export")
	keyFile := fs.String("signing-key-file", "", "file holding AUDIT_EXPORT_SIGNING_KEY (default: $AUDIT_EXPORT_SIGNING_KEY / _FILE)")
	requireSig := fs.Bool("require-signature", false, "fail when no signing key is available or the export is unsigned")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("audit verify-export <file|-> [--signing-key-file F] [--require-signature]")
	}
	key := a.cfg.AuditExportSigningKey
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			return err
		}
		key = strings.TrimSpace(string(b))
	}
	if *requireSig && key == "" {
		return usageErr("--require-signature needs a signing key (--signing-key-file or AUDIT_EXPORT_SIGNING_KEY)")
	}
	r := io.Reader(a.stdin)
	if pos[0] != "-" {
		f, err := os.Open(pos[0])
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	res, err := auditchain.VerifyExport(r, key)
	if err != nil {
		return err
	}
	if a.jsonOut {
		if err := a.printJSON(res); err != nil {
			return err
		}
	} else {
		t := res.Trailer
		mark := "✓"
		if !res.Valid {
			mark = "✗"
		}
		a.outf("%s export %s: tenant %s, %d entries (seq %d..%d), head %s\n",
			mark, map[bool]string{true: "VALID", false: "INVALID"}[res.Valid], t.TenantID, res.Entries, t.FirstSeq, t.LastSeq, t.HeadHash)
		a.outf("  chain links+hashes: %s\n  content digest:     %s\n  trailer summary:    %s\n",
			okStr(res.ChainValid), okStr(res.ContentHashOK), okStr(res.TrailerMatches))
		switch {
		case res.SignatureChecked:
			a.outf("  HMAC signature:     %s\n", okStr(res.SignatureOK))
		case t.Signature != "":
			a.outln("  HMAC signature:     present, not checked (no key supplied)")
		default:
			a.outln("  HMAC signature:     unsigned export")
		}
		if !t.Complete {
			a.outf("  partial export: resume with --from-seq %d\n", t.NextFromSeq)
		}
		for _, p := range res.Problems {
			a.outf("  problem: %s\n", p)
		}
	}
	if !res.Valid || (*requireSig && !res.SignatureOK) {
		return errInvalid
	}
	return nil
}

func okStr(ok bool) string {
	if ok {
		return "ok"
	}
	return "FAILED"
}

// ─── version ───────────────────────────────────────────────────────────────

func (a *app) cmdVersion(args []string) error {
	if _, err := parseInterspersed(a.newFlags("version"), args); err != nil {
		return err
	}
	a.outf("pcmi CLI %s\n", version.Tag)
	var health struct {
		Version string `json:"version"`
	}
	if err := a.client.json(a.ctx, "GET", "/v1/health", nil, &health); err != nil {
		a.outf("server %s: unreachable (%v)\n", a.client.base, err)
		return nil
	}
	a.outf("server %s: %s\n", a.client.base, health.Version)
	return nil
}
