package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

type fakeRetentionStore struct {
	policies  []model.RetentionPolicy
	counts    model.EraseCounts
	vertexIDs []string
	err       error
	graphErr  error
	graphN    int

	gotErase   *model.EraseRequest
	gotGraph   []string
	graphCalls int
	deleted    string
}

func (f *fakeRetentionStore) List(context.Context, string) ([]model.RetentionPolicy, error) {
	return f.policies, f.err
}

func (f *fakeRetentionStore) Upsert(_ context.Context, tenantID string, req model.RetentionPolicyRequest) (model.RetentionPolicy, error) {
	if f.err != nil {
		return model.RetentionPolicy{}, f.err
	}
	return model.RetentionPolicy{TenantID: tenantID, PathPrefix: req.PathPrefix, MaxAgeDays: req.MaxAgeDays,
		SupersededRetentionDays: req.SupersededRetentionDays}, nil
}

func (f *fakeRetentionStore) Delete(_ context.Context, _ string, prefix string) error {
	f.deleted = prefix
	return f.err
}

func (f *fakeRetentionStore) Erase(_ context.Context, _ string, req model.EraseRequest, _ string) (model.EraseCounts, []string, error) {
	f.gotErase = &req
	if f.err != nil {
		return model.EraseCounts{}, nil, f.err
	}
	if req.DryRun {
		return f.counts, nil, nil
	}
	return f.counts, f.vertexIDs, nil
}

func (f *fakeRetentionStore) EraseGraphVertices(_ context.Context, _ string, ids []string) (int, error) {
	f.graphCalls++
	f.gotGraph = ids
	return f.graphN, f.graphErr
}

func days(n int) *int { return &n }

func TestRetentionService_UpsertValidates(t *testing.T) {
	svc := NewRetentionService(&fakeRetentionStore{})
	if _, err := svc.Upsert(context.Background(), "t", model.RetentionPolicyRequest{PathPrefix: "a"}); !errors.Is(err, ErrInvalidRetentionRequest) {
		t.Fatalf("want ErrInvalidRetentionRequest, got %v", err)
	}
	p, err := svc.Upsert(context.Background(), "t", model.RetentionPolicyRequest{PathPrefix: " a.b ", MaxAgeDays: days(30)})
	if err != nil || p.PathPrefix != "a.b" || *p.MaxAgeDays != 30 {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}

func TestRetentionService_DeleteValidates(t *testing.T) {
	st := &fakeRetentionStore{}
	svc := NewRetentionService(st)
	if err := svc.Delete(context.Background(), "t", "bad..x"); !errors.Is(err, ErrInvalidRetentionRequest) {
		t.Fatalf("got %v", err)
	}
	if err := svc.Delete(context.Background(), "t", ""); err != nil || st.deleted != "" {
		t.Fatalf("tenant-wide delete: %v", err)
	}
}

func TestRetentionService_ListPassesThrough(t *testing.T) {
	st := &fakeRetentionStore{policies: []model.RetentionPolicy{{ID: 1}}}
	got, err := NewRetentionService(st).List(context.Background(), "t")
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestRetentionService_EraseDryRunSkipsGraph(t *testing.T) {
	st := &fakeRetentionStore{counts: model.EraseCounts{MemoryVersions: 4}}
	res, err := NewRetentionService(st).Erase(context.Background(), "t", model.EraseRequest{PathPrefix: "u.a", DryRun: true}, "")
	if err != nil || !res.DryRun || res.Counts.MemoryVersions != 4 || res.ErasedAt != nil || st.graphCalls != 0 {
		t.Fatalf("res=%+v err=%v graphCalls=%d", res, err, st.graphCalls)
	}
}

func TestRetentionService_EraseRunsGraphCleanup(t *testing.T) {
	st := &fakeRetentionStore{counts: model.EraseCounts{MemoryVersions: 2}, vertexIDs: []string{"u.a", "memory.7"}, graphN: 2}
	svc := NewRetentionService(st)
	svc.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	res, err := svc.Erase(context.Background(), "t", model.EraseRequest{PathPrefix: "u.a", Reason: "dsr"}, "key")
	if err != nil || res.GraphVertices != 2 || res.GraphError != "" || res.ErasedAt == nil || len(st.gotGraph) != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRetentionService_EraseGraphErrorIsReportedNotFatal(t *testing.T) {
	st := &fakeRetentionStore{vertexIDs: []string{"x"}, graphErr: errors.New("age down")}
	res, err := NewRetentionService(st).Erase(context.Background(), "t", model.EraseRequest{PathPrefix: "x"}, "")
	if err != nil || res.GraphError != "age down" || res.ErasedAt == nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRetentionService_EraseErrors(t *testing.T) {
	if _, err := NewRetentionService(&fakeRetentionStore{}).Erase(context.Background(), "t", model.EraseRequest{}, ""); !errors.Is(err, ErrInvalidRetentionRequest) {
		t.Fatalf("empty prefix: %v", err)
	}
	if _, err := NewRetentionService(&fakeRetentionStore{err: errors.New("db")}).Erase(context.Background(), "t", model.EraseRequest{PathPrefix: "a"}, ""); err == nil || errors.Is(err, ErrInvalidRetentionRequest) {
		t.Fatalf("store error: %v", err)
	}
}
