package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/auditchain"
)

const auditTenant = "22222222-2222-2222-2222-222222222222"

type fakeChain struct {
	recs    []auditchain.Record
	err     error
	headErr error
	calls   int
}

func (f *fakeChain) ChainBatch(_ context.Context, _ string, after, to int64, limit int) ([]auditchain.Record, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out []auditchain.Record
	for _, r := range f.recs {
		if r.ChainSeq <= after || (to > 0 && r.ChainSeq > to) {
			continue
		}
		out = append(out, r)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeChain) ChainHead(context.Context, string) (int64, string, error) {
	if f.headErr != nil {
		return 0, "", f.headErr
	}
	if len(f.recs) == 0 {
		return 0, "", nil
	}
	last := f.recs[len(f.recs)-1]
	return last.ChainSeq, last.RowHash, nil
}

func chainOf(n int) []auditchain.Record {
	prev := auditchain.GenesisHash
	out := make([]auditchain.Record, 0, n)
	for i := 1; i <= n; i++ {
		f := auditchain.Fields{TenantID: auditTenant, ChainSeq: int64(i), EventType: "api_request",
			Path: "/v1/x", Method: "GET", StatusCode: 200, CreatedAt: "2026-09-24T00:00:00.000000Z"}
		h := auditchain.Hash(prev, auditchain.Canonical(f))
		out = append(out, auditchain.Record{Fields: f, ID: int64(i), PrevHash: prev, RowHash: h})
		prev = h
	}
	return out
}

func TestAuditService_VerifyValidAcrossBatches(t *testing.T) {
	src := &fakeChain{recs: chainOf(auditChainBatchSize + 5)}
	res, err := NewAuditService(src, "").Verify(context.Background(), auditTenant)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid || res.Checked != int64(auditChainBatchSize+5) || res.HeadSeq != int64(auditChainBatchSize+5) {
		t.Fatalf("got %+v", res)
	}
	if src.calls != 2 {
		t.Fatalf("expected 2 batches, got %d", src.calls)
	}
	if res.Algorithm != auditchain.Algorithm || res.VerifiedAt.IsZero() {
		t.Fatalf("metadata: %+v", res)
	}
}

func TestAuditService_VerifyDetectsTamper(t *testing.T) {
	recs := chainOf(4)
	recs[2].UserAgent = "forged"
	res, err := NewAuditService(&fakeChain{recs: recs}, "").Verify(context.Background(), auditTenant)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid || res.FirstBreak == nil || res.FirstBreak.ChainSeq != 3 {
		t.Fatalf("got %+v", res)
	}
}

func TestAuditService_VerifyPropagatesError(t *testing.T) {
	if _, err := NewAuditService(&fakeChain{err: errors.New("db")}, "").Verify(context.Background(), auditTenant); err == nil {
		t.Fatal("expected error")
	}
}

func TestAuditService_ExportFullSigned(t *testing.T) {
	svc := NewAuditService(&fakeChain{recs: chainOf(3)}, "key")
	svc.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	if !svc.SigningEnabled() {
		t.Fatal("signing should be enabled")
	}
	var buf bytes.Buffer
	tr, err := svc.Export(context.Background(), auditTenant, AuditExportOptions{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Complete || tr.Count != 3 || tr.NextFromSeq != 0 || tr.Signature == "" {
		t.Fatalf("trailer: %+v", tr)
	}
	chk, err := auditchain.VerifyExport(&buf, "key")
	if err != nil || !chk.Valid {
		t.Fatalf("export does not verify: %+v %v", chk, err)
	}
}

func TestAuditService_ExportPaginates(t *testing.T) {
	svc := NewAuditService(&fakeChain{recs: chainOf(10)}, "")
	var buf bytes.Buffer
	tr, err := svc.Export(context.Background(), auditTenant, AuditExportOptions{FromSeq: 3, Limit: 4}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Complete || tr.FirstSeq != 3 || tr.LastSeq != 6 || tr.NextFromSeq != 7 || tr.Count != 4 {
		t.Fatalf("trailer: %+v", tr)
	}
	chk, _ := auditchain.VerifyExport(&buf, "")
	if !chk.Valid {
		t.Fatalf("partial export must verify from its anchor: %+v", chk)
	}
}

func TestAuditService_ExportToSeqIsComplete(t *testing.T) {
	var buf bytes.Buffer
	tr, err := NewAuditService(&fakeChain{recs: chainOf(10)}, "").
		Export(context.Background(), auditTenant, AuditExportOptions{FromSeq: 2, ToSeq: 5}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Complete || tr.LastSeq != 5 || tr.Count != 4 {
		t.Fatalf("trailer: %+v", tr)
	}
}

func TestAuditService_ExportEmpty(t *testing.T) {
	var buf bytes.Buffer
	tr, err := NewAuditService(&fakeChain{}, "").Export(context.Background(), auditTenant, AuditExportOptions{}, &buf)
	if err != nil || !tr.Complete || tr.Count != 0 {
		t.Fatalf("tr=%+v err=%v", tr, err)
	}
}

func TestAuditService_ExportErrors(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	for _, o := range []AuditExportOptions{
		{FromSeq: -1}, {ToSeq: -1}, {FromSeq: 5, ToSeq: 2}, {Limit: -1}, {Limit: AuditExportMaxLimit + 1},
	} {
		if _, err := NewAuditService(&fakeChain{}, "").Export(ctx, auditTenant, o, &buf); !errors.Is(err, ErrAuditExportRange) {
			t.Fatalf("opts %+v: want ErrAuditExportRange, got %v", o, err)
		}
	}
	if _, err := NewAuditService(&fakeChain{headErr: errors.New("h")}, "").Export(ctx, auditTenant, AuditExportOptions{}, &buf); err == nil {
		t.Fatal("expected head error")
	}
	src := &fakeChain{recs: chainOf(2)}
	src.err = errors.New("batch")
	if _, err := NewAuditService(src, "").Export(ctx, auditTenant, AuditExportOptions{}, &buf); err == nil {
		t.Fatal("expected batch error")
	}
}
