package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestExtractiveSummaryShort(t *testing.T) {
	parts := []string{"hello", "world"}
	got := extractiveSummary(parts, "")
	if got != "hello\n\nworld" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestExtractiveSummaryTruncated(t *testing.T) {
	// Build a string longer than 400 characters
	longPart := strings.Repeat("a", 500)
	got := extractiveSummary([]string{longPart}, "")
	if len(got) > 405 { // 400 + "…"
		t.Fatalf("summary not truncated: len=%d", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis at end of truncated summary, got: %q", got[:20])
	}
}

func TestExtractiveSummaryDetailed(t *testing.T) {
	longPart := strings.Repeat("b", 1100)
	got := extractiveSummary([]string{longPart}, "detailed")
	// 1200 chars limit for detailed
	if len(got) > 1205 {
		t.Fatalf("detailed summary too long: %d", len(got))
	}
}

func TestExtractiveSummaryDetailedNoTrunc(t *testing.T) {
	part := strings.Repeat("c", 50)
	got := extractiveSummary([]string{part}, "Detailed") // case-insensitive
	if got != part {
		t.Fatalf("expected no truncation for short detailed summary")
	}
}

func TestExtractiveSummaryEmpty(t *testing.T) {
	got := extractiveSummary(nil, "")
	if got != "" {
		t.Fatalf("expected empty summary for nil parts, got %q", got)
	}
}

func TestSummarizeMethod_extractiveFallback(t *testing.T) {
	repo := &summarizeMemRepoStub{
		entries: []model.MemoryEntry{{ID: 1, Content: "hello world"}},
	}
	t.Setenv("OPENAI_API_KEY", "")
	svc := NewSummarizeService(repo)
	res, err := svc.Summarize(context.Background(), &SummarizeRequest{PathPrefix: "root.x", Limit: 5}, "tid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != "extractive" || res.Total != 1 || res.Summary == "" {
		t.Fatalf("%+v", res)
	}
}

func TestSummarizeMethod_emptyRetrieve(t *testing.T) {
	repo := &summarizeMemRepoStub{entries: []model.MemoryEntry{}}
	svc := NewSummarizeService(repo)
	res, err := svc.Summarize(context.Background(), &SummarizeRequest{}, "tid")
	if err != nil || res.Method != "none" || res.Total != 0 {
		t.Fatalf("%+v err=%v", res, err)
	}
}

func TestSummarizeMethod_retrieveErr(t *testing.T) {
	repo := &summarizeMemRepoStub{err: errors.New("boom")}
	svc := NewSummarizeService(repo)
	_, err := svc.Summarize(context.Background(), &SummarizeRequest{PathPrefix: "p"}, "tid")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewSummarizeService(t *testing.T) {
	if NewSummarizeService(&summarizeMemRepoStub{}) == nil {
		t.Fatal("nil svc")
	}
}

type summarizeMemRepoStub struct {
	entries []model.MemoryEntry
	err     error
}

func (s *summarizeMemRepoStub) Store(context.Context, model.StoreRequest, string) (int64, int, *int64, error) {
	return 0, 0, nil, nil
}

func (s *summarizeMemRepoStub) Retrieve(_ context.Context, _ model.RetrieveRequest, _ string, _ []float32) ([]model.MemoryEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

func (s *summarizeMemRepoStub) GetHistoricalVersion(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, nil
}

func (s *summarizeMemRepoStub) GetByPath(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, nil
}

func (s *summarizeMemRepoStub) ExportMemories(context.Context, string, string, int, bool) ([]model.MemoryEntry, error) {
	return nil, nil
}

func (s *summarizeMemRepoStub) CompactPathHistory(context.Context, string, string, int) (int, error) {
	return 0, nil
}
