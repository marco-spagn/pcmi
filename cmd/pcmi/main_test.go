package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/auditchain"
	"github.com/marco-spagn/pcmi/internal/config"
)

// fakeAPI records requests and serves canned responses.
type fakeAPI struct {
	mu       sync.Mutex
	requests []recorded
	chainOK  bool
	export   []byte
	partial  bool
}

type recorded struct {
	Method, Path, Query, APIKey string
	Body                        map[string]any
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("X-API-Key"), body})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/memories" && r.Method == "POST":
			if body["path"] == "root.fail" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid path"}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":7,"version":1,"status":"created","path":%q}`, body["path"])
		case strings.HasPrefix(r.URL.Path, "/v1/memories/") && r.Method == "GET":
			_, _ = w.Write([]byte(`{"id":7,"path":"root.a","content":"hello world","version":2,"valid_from":"2026-09-01T00:00:00Z"}`))
		case r.URL.Path == "/v1/retrieve":
			_, _ = w.Write([]byte(`{"entries":[{"id":1,"path":"root.a","content":"alpha   beta","version":1,"relevance_score":0.91},{"id":2,"path":"root.b","content":"gamma","version":3}],"total":2,"reranked":true,"has_more":true}`))
		case r.URL.Path == "/v1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fl, _ := w.(http.Flusher)
			_, _ = io.WriteString(w, ": heartbeat 1\n\n")
			_, _ = io.WriteString(w, "event: memory.stored\ndata: {\"type\":\"memory.stored\",\"payload\":{\"path\":\"root.a\"}}\n\n")
			_, _ = io.WriteString(w, "event: memory.updated\ndata: {\"type\":\"memory.updated\",\"payload\":{\"path\":\"root.b\"}}\n\n")
			if fl != nil {
				fl.Flush()
			}
			<-r.Context().Done()
		case r.URL.Path == "/v1/stats/usage":
			_, _ = w.Write([]byte(`{"from":"2026-09-01","to":"2026-09-30","rows":[{"operation":"embedding","provider":"openai","model":"emb","requests":3,"input_tokens":1000,"output_tokens":0,"estimated_cost_usd":0.002},{"operation":"rerank","model":"claude","requests":1,"input_tokens":10,"output_tokens":5,"estimated_cost_usd":null}],"totals":{"requests":4,"input_tokens":1010,"output_tokens":5,"estimated_cost_usd":null},"unpriced_models":["claude"]}`))
		case r.URL.Path == "/v1/memories/erase":
			_, _ = w.Write([]byte(`{"path_prefix":"users.alice","dry_run":true,"counts":{"memory_versions":3,"paths":2},"graph_error":"age down"}`))
		case r.URL.Path == "/v1/audit/verify":
			if f.chainOK {
				_, _ = w.Write([]byte(`{"valid":true,"checked":5,"head_seq":5,"head_hash":"abc"}`))
			} else {
				_, _ = w.Write([]byte(`{"valid":false,"checked":5,"head_seq":5,"first_break":{"chain_seq":3,"reason":"hash_mismatch"}}`))
			}
		case r.URL.Path == "/v1/audit/export":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("X-PCMI-Audit-Head-Hash", "headhash")
			w.Header().Set("X-PCMI-Audit-Complete", fmt.Sprint(!f.partial))
			_, _ = w.Write(f.export)
		case r.URL.Path == "/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok","version":"v9.9.9"}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *fakeAPI) last() recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func (f *fakeAPI) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.Path == path {
			n++
		}
	}
	return n
}

type result struct {
	code           int
	stdout, stderr string
}

func runCLI(t *testing.T, srvURL string, cfg *config.Config, stdin string, args ...string) result {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.PCMIBaseURL = srvURL
	if cfg.PCMIAPIKey == "" {
		cfg.PCMIAPIKey = "test-key"
	}
	var out, errb bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := &app{ctx: ctx, stdin: strings.NewReader(stdin), stdout: &out, stderr: &errb, cfg: cfg}
	code := a.run(args)
	return result{code, out.String(), errb.String()}
}

func newFake(t *testing.T) (*fakeAPI, *httptest.Server) {
	t.Helper()
	f := &fakeAPI{chainOK: true}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return f, srv
}

func TestStore(t *testing.T) {
	f, srv := newFake(t)
	r := runCLI(t, srv.URL, nil, "", "store", "root.a", "hello", "--tag", "x", "--tag", "y", "--meta", "n=3", "--meta", "s=text", "--importance", "0.9")
	if r.code != 0 || !strings.Contains(r.stdout, "stored root.a  id=7 version=1") {
		t.Fatalf("%+v", r)
	}
	req := f.last()
	meta := req.Body["metadata"].(map[string]any)
	if req.APIKey != "test-key" || req.Body["content"] != "hello" || meta["n"] != float64(3) || meta["s"] != "text" ||
		fmt.Sprint(req.Body["tags"]) != "[x y]" || req.Body["importance"] != 0.9 {
		t.Fatalf("request %+v", req)
	}

	r = runCLI(t, srv.URL, nil, "from stdin\n", "store", "root.b", "-")
	if r.code != 0 || f.last().Body["content"] != "from stdin" {
		t.Fatalf("stdin: %+v %+v", r, f.last())
	}
	r = runCLI(t, srv.URL, nil, "piped", "--json", "store", "root.c")
	if r.code != 0 || !strings.Contains(r.stdout, `"status": "created"`) {
		t.Fatalf("json: %+v", r)
	}
}

func TestStoreErrors(t *testing.T) {
	_, srv := newFake(t)
	for name, tc := range map[string]struct {
		stdin string
		args  []string
		code  int
		msg   string
	}{
		"no args":      {"", []string{"store"}, 2, ""},
		"empty":        {"  ", []string{"store", "root.a"}, 2, "content is empty"},
		"bad meta":     {"", []string{"store", "root.a", "x", "--meta", "novalue"}, 2, "key=value"},
		"unknown flag": {"", []string{"store", "--bogus", "root.a", "x"}, 2, ""},
		"api error":    {"", []string{"store", "root.fail", "x"}, 1, "HTTP 400: invalid path"},
	} {
		t.Run(name, func(t *testing.T) {
			r := runCLI(t, srv.URL, nil, tc.stdin, tc.args...)
			if r.code != tc.code || !strings.Contains(r.stderr, tc.msg) {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestGet(t *testing.T) {
	f, srv := newFake(t)
	r := runCLI(t, srv.URL, nil, "", "get", "root.a", "--as-of", "2026-09-01T10:00:00+02:00")
	if r.code != 0 || !strings.Contains(r.stdout, "root.a  v2") || !strings.Contains(r.stdout, "hello world") {
		t.Fatalf("%+v", r)
	}
	if req := f.last(); req.Path != "/v1/memories/root.a" || req.Query != "as_of=2026-09-01T10%3A00%3A00%2B02%3A00" {
		t.Fatalf("request %+v", req)
	}
	r = runCLI(t, srv.URL, nil, "", "--json", "get", "--version", "1", "root.a")
	if r.code != 0 || f.last().Query != "version=1" || !strings.Contains(r.stdout, `"content":"hello world"`) {
		t.Fatalf("%+v %+v", r, f.last())
	}
	if r := runCLI(t, srv.URL, nil, "", "get"); r.code != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestRetrieve(t *testing.T) {
	f, srv := newFake(t)
	r := runCLI(t, srv.URL, nil, "", "retrieve", "what", "broke", "--prefix", "root", "--limit", "5", "--no-rerank", "--tag", "incident", "--as-of", "2026-01-01T00:00:00Z")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"SCORE", "0.910", "root.a", "alpha beta", "reranked by LLM", "more results"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in\n%s", want, r.stdout)
		}
	}
	b := f.last().Body
	if b["query"] != "what broke" || b["path_prefix"] != "root" || b["limit"] != float64(5) || b["rerank"] != false ||
		b["as_of"] != "2026-01-01T00:00:00Z" || fmt.Sprint(b["tags"]) != "[incident]" {
		t.Fatalf("body %+v", b)
	}
	r = runCLI(t, srv.URL, nil, "", "--json", "search", "-q", "x")
	if r.code != 0 || !strings.Contains(r.stdout, `"reranked":true`) {
		t.Fatalf("%+v", r)
	}
	if _, ok := f.last().Body["rerank"]; ok {
		t.Fatal("rerank must be omitted unless --no-rerank")
	}
}

func TestRetrieveEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"entries":[],"total":0}`))
	}))
	defer srv.Close()
	if r := runCLI(t, srv.URL, nil, "", "retrieve", "-q", "x"); r.code != 0 || !strings.Contains(r.stdout, "no memories matched") {
		t.Fatalf("%+v", r)
	}
}

func TestTail(t *testing.T) {
	f, srv := newFake(t)
	r := runCLI(t, srv.URL, nil, "", "tail", "-n", "2", "--types", "memory.stored,memory.updated")
	if r.code != 0 || !strings.Contains(r.stdout, "memory.stored") || !strings.Contains(r.stdout, "path=root.b") {
		t.Fatalf("%+v", r)
	}
	if q := f.last().Query; q != "types=memory.stored%2Cmemory.updated" {
		t.Fatalf("query %q", q)
	}
	r = runCLI(t, srv.URL, nil, "", "--json", "tail", "-n", "1")
	if r.code != 0 || strings.TrimSpace(r.stdout) != `{"type":"memory.stored","payload":{"path":"root.a"}}` {
		t.Fatalf("%+v", r)
	}
}

func TestTailHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	if r := runCLI(t, srv.URL, nil, "", "tail"); r.code != 1 || !strings.Contains(r.stderr, "HTTP 401") {
		t.Fatalf("%+v", r)
	}
}

func TestSeed(t *testing.T) {
	f, srv := newFake(t)
	r := runCLI(t, srv.URL, nil, "", "seed", "--prefix", "root.try.")
	if r.code != 0 || !strings.Contains(r.stdout, fmt.Sprintf("stored %d memories", len(demoCorpus))) || !strings.Contains(r.stdout, "pcmi retrieve --prefix root.try") {
		t.Fatalf("%+v", r)
	}
	if f.count("/v1/memories") != len(demoCorpus) || !strings.HasPrefix(f.last().Body["path"].(string), "root.try.") {
		t.Fatalf("requests %d last %+v", f.count("/v1/memories"), f.last())
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "m.jsonl")
	_ = os.WriteFile(good, []byte("# comment\n{\"path\":\"root.f.a\",\"content\":\"one\",\"tags\":[\"t\"],\"importance\":0.2,\"metadata\":{\"k\":1}}\n\n{\"path\":\"root.f.b\",\"content\":\"two\"}\n"), 0o600)
	r = runCLI(t, srv.URL, nil, "", "seed", "--file", good)
	if r.code != 0 || !strings.Contains(r.stdout, "stored 2 memories") || strings.Contains(r.stdout, "Try:") {
		t.Fatalf("%+v", r)
	}
	bad := filepath.Join(dir, "bad.jsonl")
	_ = os.WriteFile(bad, []byte("{\"path\":\"\",\"content\":\"x\"}\n"), 0o600)
	if r := runCLI(t, srv.URL, nil, "", "seed", "--file", bad); r.code != 1 || !strings.Contains(r.stderr, "line 1") {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, srv.URL, nil, "", "seed", "--file", filepath.Join(dir, "missing")); r.code != 1 {
		t.Fatalf("%+v", r)
	}
	_ = os.WriteFile(bad, []byte("{not json\n"), 0o600)
	if r := runCLI(t, srv.URL, nil, "", "seed", "--file", bad); r.code != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestUsage(t *testing.T) {
	f, srv := newFake(t)
	r := runCLI(t, srv.URL, nil, "", "usage", "--from", "2026-09-01", "--group-by", "operation")
	if r.code != 0 || !strings.Contains(r.stdout, "$0.0020") || !strings.Contains(r.stdout, "TOTAL") ||
		!strings.Contains(r.stdout, "n/a") || !strings.Contains(r.stdout, "no LLM_PRICING entry for: claude") {
		t.Fatalf("%+v", r)
	}
	if q := f.last().Query; q != "from=2026-09-01&group_by=operation" {
		t.Fatalf("query %q", q)
	}
	if r := runCLI(t, srv.URL, nil, "", "--json", "usage"); r.code != 0 || !strings.Contains(r.stdout, `"unpriced_models"`) {
		t.Fatalf("%+v", r)
	}
}

func TestErase(t *testing.T) {
	f, srv := newFake(t)
	if r := runCLI(t, srv.URL, nil, "", "erase", "users.alice"); r.code != 2 || !strings.Contains(r.stderr, "--dry-run first") {
		t.Fatalf("must refuse without --yes: %+v", r)
	}
	if f.count("/v1/memories/erase") != 0 {
		t.Fatal("no request may be sent without confirmation")
	}
	r := runCLI(t, srv.URL, nil, "", "erase", "users.alice", "--dry-run", "--reason", "DSR-1")
	if r.code != 0 || !strings.Contains(r.stdout, "would erase under users.alice") || !strings.Contains(r.stdout, "graph cleanup FAILED: age down") {
		t.Fatalf("%+v", r)
	}
	if b := f.last().Body; b["dry_run"] != true || b["reason"] != "DSR-1" {
		t.Fatalf("body %+v", b)
	}
	r = runCLI(t, srv.URL, nil, "", "--json", "erase", "--yes", "users.alice")
	if r.code != 0 || f.last().Body["dry_run"] != false || !strings.Contains(r.stdout, `"counts"`) {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, srv.URL, nil, "", "erase"); r.code != 2 {
		t.Fatalf("%+v", r)
	}
}

func buildExport(t *testing.T, key string, tamper bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := auditchain.NewWriter(&buf, "tenant-x")
	prev := auditchain.GenesisHash
	for i := int64(1); i <= 3; i++ {
		f := auditchain.Fields{TenantID: "tenant-x", ChainSeq: i, EventType: "api_request", Path: "/v1/x", Method: "GET", StatusCode: 200}
		h := auditchain.Hash(prev, auditchain.Canonical(f))
		rec := auditchain.Record{Fields: f, PrevHash: prev, RowHash: h}
		if tamper && i == 2 {
			rec.Path = "/v1/forged"
		}
		if err := w.Write(rec); err != nil {
			t.Fatal(err)
		}
		prev = h
	}
	if _, err := w.Close(time.Now(), true, 0, key); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAuditVerify(t *testing.T) {
	f, srv := newFake(t)
	if r := runCLI(t, srv.URL, nil, "", "audit", "verify"); r.code != 0 || !strings.Contains(r.stdout, "✓ audit chain intact: 5 rows") {
		t.Fatalf("%+v", r)
	}
	f.chainOK = false
	if r := runCLI(t, srv.URL, nil, "", "audit", "verify"); r.code != 1 || !strings.Contains(r.stdout, "BROKEN at seq 3 (hash_mismatch)") || r.stderr != "" {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, srv.URL, nil, "", "--json", "audit", "verify"); r.code != 1 || !strings.Contains(r.stdout, `"valid":false`) {
		t.Fatalf("%+v", r)
	}
	for _, args := range [][]string{{"audit"}, {"audit", "nope"}} {
		if r := runCLI(t, srv.URL, nil, "", args...); r.code != 2 {
			t.Fatalf("%v: %+v", args, r)
		}
	}
}

func TestAuditExportAndVerifyExport(t *testing.T) {
	f, srv := newFake(t)
	f.export = buildExport(t, "s3cret", false)
	f.partial = true
	out := filepath.Join(t.TempDir(), "audit.jsonl")
	r := runCLI(t, srv.URL, nil, "", "audit", "export", "-o", out, "--from-seq", "2", "--to-seq", "9", "--limit", "5")
	if r.code != 0 || !strings.Contains(r.stderr, "wrote "+out) || !strings.Contains(r.stderr, "export is partial") {
		t.Fatalf("%+v", r)
	}
	if q := f.last().Query; q != "from_seq=2&limit=5&to_seq=9" {
		t.Fatalf("query %q", q)
	}
	if r := runCLI(t, srv.URL, nil, "", "audit", "export"); r.code != 0 || !strings.Contains(r.stdout, `"type":"trailer"`) {
		t.Fatalf("stdout export: %+v", r)
	}

	keyFile := filepath.Join(t.TempDir(), "key")
	_ = os.WriteFile(keyFile, []byte("s3cret\n"), 0o600)
	r = runCLI(t, srv.URL, nil, "", "audit", "verify-export", out, "--signing-key-file", keyFile, "--require-signature")
	if r.code != 0 || !strings.Contains(r.stdout, "✓ export VALID") || !strings.Contains(r.stdout, "HMAC signature:     ok") {
		t.Fatalf("%+v", r)
	}
	// Key from config (AUDIT_EXPORT_SIGNING_KEY) and stdin input.
	r = runCLI(t, srv.URL, &config.Config{AuditExportSigningKey: "wrong"}, string(f.export), "audit", "verify-export", "-")
	if r.code != 1 || !strings.Contains(r.stdout, "HMAC signature:     FAILED") {
		t.Fatalf("%+v", r)
	}
	// No key: signature present but unchecked, still valid.
	r = runCLI(t, srv.URL, nil, string(f.export), "audit", "verify-export", "-")
	if r.code != 0 || !strings.Contains(r.stdout, "present, not checked") {
		t.Fatalf("%+v", r)
	}
	// Tampered export.
	r = runCLI(t, srv.URL, nil, string(buildExport(t, "", true)), "--json", "audit", "verify-export", "-")
	if r.code != 1 || !strings.Contains(r.stdout, `"chain_valid": false`) {
		t.Fatalf("%+v", r)
	}
	// Unsigned export in human mode + --require-signature without key.
	r = runCLI(t, srv.URL, nil, string(buildExport(t, "", false)), "audit", "verify-export", "-")
	if r.code != 0 || !strings.Contains(r.stdout, "unsigned export") {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, srv.URL, nil, "", "audit", "verify-export", out, "--require-signature"); r.code != 2 {
		t.Fatalf("%+v", r)
	}
	for _, args := range [][]string{
		{"audit", "verify-export"},
		{"audit", "verify-export", "/no/such/file"},
		{"audit", "verify-export", out, "--signing-key-file", "/no/such/key"},
	} {
		if r := runCLI(t, srv.URL, nil, "", args...); r.code == 0 {
			t.Fatalf("%v should fail: %+v", args, r)
		}
	}
	if r := runCLI(t, srv.URL, nil, "garbage\n", "audit", "verify-export", "-"); r.code != 1 {
		t.Fatalf("garbage: %+v", r)
	}
}

func TestVersionHelpAndGlobals(t *testing.T) {
	_, srv := newFake(t)
	if r := runCLI(t, srv.URL, nil, "", "version"); r.code != 0 || !strings.Contains(r.stdout, "v9.9.9") {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, "http://127.0.0.1:1", nil, "", "--timeout", "500ms", "version"); r.code != 0 || !strings.Contains(r.stdout, "unreachable") {
		t.Fatalf("%+v", r)
	}
	for _, args := range [][]string{{"help"}, {"--help"}} {
		if r := runCLI(t, srv.URL, nil, "", args...); r.code != 0 {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	for _, args := range [][]string{{}, {"frobnicate"}, {"--nope"}} {
		if r := runCLI(t, srv.URL, nil, "", args...); r.code != 2 {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	// --url / --api-key override config.
	f2, srv2 := newFake(t)
	if r := runCLI(t, srv.URL, nil, "", "--url", srv2.URL+"/", "--api-key", "other", "retrieve", "-q", "x"); r.code != 0 || f2.last().APIKey != "other" {
		t.Fatalf("%+v", r)
	}
}

func TestSnippetAndHelpers(t *testing.T) {
	if got := snippet("a  b\n c", 10); got != "a b c" {
		t.Fatalf("%q", got)
	}
	if got := snippet(strings.Repeat("é", 20), 5); got != "éééé…" {
		t.Fatalf("%q", got)
	}
	if firstNonEmpty(" ", "", " x ") != "x" || firstNonEmpty() != "" {
		t.Fatal("firstNonEmpty")
	}
	var m multiFlag
	_ = m.Set("a")
	_ = m.Set("b")
	if m.String() != "a,b" {
		t.Fatal(m.String())
	}
}
