package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/usage"
)

type rerankLLM struct {
	mu         sync.Mutex
	configured bool
	answer     string
	err        error
	block      bool // wait for ctx cancellation
	calls      int
	lastPrompt string
	lastTenant string
	lastOp     string
}

func (l *rerankLLM) IsConfigured() bool { return l.configured }

func (l *rerankLLM) Complete(ctx context.Context, _ string, msgs []string) (string, error) {
	l.mu.Lock()
	l.calls++
	l.lastPrompt = strings.Join(msgs, "\n")
	l.lastTenant, l.lastOp = usage.ScopeFrom(ctx)
	l.mu.Unlock()
	if l.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return l.answer, l.err
}

func entriesN(n int) []model.MemoryEntry {
	out := make([]model.MemoryEntry, n)
	for i := range out {
		out[i] = model.MemoryEntry{ID: int64(i + 1), Path: fmt.Sprintf("root.m%d", i), Content: fmt.Sprintf("content %d", i)}
	}
	return out
}

func ids(es []model.MemoryEntry) []int64 {
	out := make([]int64, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

func TestParseRanking(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want []int
		err  bool
	}{
		"wrapped ints":       {`{"ranking":[2,0,1]}`, []int{2, 0, 1}, false},
		"objects":            {`{"ranking":[{"index":1,"score":0.9},{"index":2}]}`, []int{1, 2, 0}, false},
		"bare array":         {`[1]`, []int{1, 0, 2}, false},
		"fenced":             {"```json\n{\"ranking\":[2,1,0]}\n```", []int{2, 1, 0}, false},
		"dupes and bad":      {`{"ranking":[5,-1,1,1,"x",{"i":0},0]}`, []int{1, 0, 2}, false},
		"no valid index":     {`{"ranking":[9,9]}`, nil, true},
		"garbage":            {`not json`, nil, true},
		"empty ranking list": {`{"ranking":[]}`, nil, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseRanking(tc.raw, 3)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil || fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("got %v err %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestLLMReranker_reorders(t *testing.T) {
	llm := &rerankLLM{configured: true, answer: `{"ranking":[2,0]}`}
	es := entriesN(3)
	es[1].Content = strings.Repeat("é", 1000) // multi-byte, must be truncated safely
	got, err := NewLLMReranker(llm, time.Second).Rerank(context.Background(), "tenant-r", "what happened?", es)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids(got)) != "[3 1 2]" {
		t.Fatalf("order %v", ids(got))
	}
	if !strings.Contains(llm.lastPrompt, "Query: what happened?") || !strings.Contains(llm.lastPrompt, "[2] (root.m2) content 2") {
		t.Fatalf("prompt %q", llm.lastPrompt)
	}
	if strings.Count(llm.lastPrompt, "é") > rerankSnippetBytes/2 {
		t.Fatal("snippet not truncated")
	}
	if llm.lastTenant != "tenant-r" || llm.lastOp != usage.OpRerank {
		t.Fatalf("usage scope %q %q", llm.lastTenant, llm.lastOp)
	}
}

func TestLLMReranker_shortCircuits(t *testing.T) {
	llm := &rerankLLM{configured: true}
	one := entriesN(1)
	if got, err := NewLLMReranker(llm, 0).Rerank(context.Background(), "t", "q", one); err != nil || len(got) != 1 || llm.calls != 0 {
		t.Fatal("single candidate must not call the LLM")
	}
	if _, err := NewLLMReranker(&rerankLLM{}, 0).Rerank(context.Background(), "t", "q", entriesN(2)); !errors.Is(err, ErrRerankUnavailable) {
		t.Fatalf("unconfigured: %v", err)
	}
	var nilR *LLMReranker
	if _, err := nilR.Rerank(context.Background(), "t", "q", entriesN(2)); !errors.Is(err, ErrRerankUnavailable) {
		t.Fatalf("nil reranker: %v", err)
	}
}

func TestLLMReranker_errorsAndTimeout(t *testing.T) {
	for name, llm := range map[string]*rerankLLM{
		"llm error": {configured: true, err: errors.New("503")},
		"bad json":  {configured: true, answer: "sorry"},
		"timeout":   {configured: true, block: true},
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			_, err := NewLLMReranker(llm, 50*time.Millisecond).Rerank(context.Background(), "t", "q", entriesN(3))
			if !errors.Is(err, ErrRerankUnavailable) {
				t.Fatalf("got %v", err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("timeout not enforced")
			}
		})
	}
}

func TestLLMReranker_breakerOpens(t *testing.T) {
	llm := &rerankLLM{configured: true, err: errors.New("down")}
	r := NewLLMReranker(llm, time.Second)
	for i := 0; i < rerankBreakerFailures+3; i++ {
		_, _ = r.Rerank(context.Background(), "t", "q", entriesN(2))
	}
	if llm.calls != rerankBreakerFailures {
		t.Fatalf("breaker should stop calls after %d failures, got %d", rerankBreakerFailures, llm.calls)
	}
}

// rerankRepo returns n hybrid-ordered rows and records the requested limit.
type rerankRepo struct {
	fullMockRepo
	n        int
	gotLimit int
}

func (r *rerankRepo) Retrieve(_ context.Context, req model.RetrieveRequest, _ string, _ []float32) ([]model.MemoryEntry, error) {
	r.gotLimit = req.Limit
	n := r.n
	if req.Limit < n {
		n = req.Limit
	}
	return entriesN(n), nil
}

type fixedReranker struct {
	err   error
	calls int
}

func (f *fixedReranker) Rerank(_ context.Context, _, _ string, es []model.MemoryEntry) ([]model.MemoryEntry, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([]model.MemoryEntry, len(es))
	for i := range es {
		out[i] = es[len(es)-1-i]
	}
	return out, nil
}

func TestMemoryService_RetrieveRerank(t *testing.T) {
	f := false
	for _, tc := range []struct {
		name         string
		req          model.RetrieveRequest
		rr           *fixedReranker
		wantIDs      string
		wantReranked bool
		wantFetch    int
	}{
		{"reranks widened pool then truncates", model.RetrieveRequest{Query: "q", Limit: 3}, &fixedReranker{}, "[20 19 18]", true, 20},
		{"fallback keeps hybrid order", model.RetrieveRequest{Query: "q", Limit: 3}, &fixedReranker{err: ErrRerankUnavailable}, "[1 2 3]", false, 20},
		{"request opt-out", model.RetrieveRequest{Query: "q", Limit: 3, Rerank: &f}, &fixedReranker{}, "[1 2 3]", false, 3},
		{"path-only never reranks", model.RetrieveRequest{PathPrefix: "root", Limit: 3}, &fixedReranker{}, "[1 2 3]", false, 3},
		{"cursor pages never rerank", model.RetrieveRequest{Query: "q", Limit: 3, Cursor: "c"}, &fixedReranker{}, "[1 2 3]", false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rerankRepo{n: 30}
			svc := NewMemoryService(repo, nil)
			svc.SetReranker(tc.rr, RerankConfig{})
			resp, err := svc.Retrieve(context.Background(), &tc.req, "t")
			if err != nil {
				t.Fatal(err)
			}
			got := ids(resp.Entries)
			if len(got) > 3 {
				got = got[:3]
			}
			if fmt.Sprint(got) != tc.wantIDs || resp.Reranked != tc.wantReranked || repo.gotLimit != tc.wantFetch {
				t.Fatalf("ids=%v reranked=%v fetch=%d", got, resp.Reranked, repo.gotLimit)
			}
			if tc.wantReranked && resp.Total != 3 {
				t.Fatalf("total %d", resp.Total)
			}
		})
	}
}

func TestMemoryService_SetRerankerClampsCandidates(t *testing.T) {
	svc := NewMemoryService(&rerankRepo{}, nil)
	svc.SetReranker(&fixedReranker{}, RerankConfig{Candidates: 500})
	if svc.rerankCfg.Candidates != RerankMaxCandidates {
		t.Fatalf("candidates %d", svc.rerankCfg.Candidates)
	}
	svc.SetReranker(nil, RerankConfig{})
	repo := &rerankRepo{n: 5}
	svc = NewMemoryService(repo, nil)
	resp, err := svc.Retrieve(context.Background(), &model.RetrieveRequest{Query: "q", Limit: 2}, "t")
	if err != nil || resp.Reranked || repo.gotLimit != 2 {
		t.Fatalf("no reranker: %+v %v fetch=%d", resp, err, repo.gotLimit)
	}
}
