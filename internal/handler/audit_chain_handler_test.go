package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/auditchain"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/service"
)

type stubChain struct {
	recs []auditchain.Record
	err  error
}

func (s *stubChain) ChainBatch(_ context.Context, _ string, after, to int64, limit int) ([]auditchain.Record, error) {
	if s.err != nil {
		return nil, s.err
	}
	var out []auditchain.Record
	for _, r := range s.recs {
		if r.ChainSeq > after && (to == 0 || r.ChainSeq <= to) && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *stubChain) ChainHead(context.Context, string) (int64, string, error) {
	if s.err != nil {
		return 0, "", s.err
	}
	if len(s.recs) == 0 {
		return 0, "", nil
	}
	return s.recs[len(s.recs)-1].ChainSeq, s.recs[len(s.recs)-1].RowHash, nil
}

func stubChainOf(tenantID string, n int) []auditchain.Record {
	prev := auditchain.GenesisHash
	var out []auditchain.Record
	for i := 1; i <= n; i++ {
		f := auditchain.Fields{TenantID: tenantID, ChainSeq: int64(i), EventType: "api_request", Path: "/v1/x", Method: "GET", StatusCode: 200}
		h := auditchain.Hash(prev, auditchain.Canonical(f))
		out = append(out, auditchain.Record{Fields: f, PrevHash: prev, RowHash: h})
		prev = h
	}
	return out
}

func auditChainHandler(src service.AuditChainSource) *AuditHandler {
	return &AuditHandler{chain: service.NewAuditService(src, "k")}
}

func TestAuditHandlerVerify(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	app := newTestApp(tenantID, "readonly")
	h := auditChainHandler(&stubChain{recs: stubChainOf(tenantID, 3)})
	app.Get("/audit/verify", h.Verify)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit/verify", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body service.AuditVerifyResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !body.Valid || body.Checked != 3 || body.TenantID != tenantID {
		t.Fatalf("status=%d body=%+v", resp.StatusCode, body)
	}
}

func TestAuditHandlerVerify_error(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	app := newTestApp(tenantID, "admin")
	app.Get("/audit/verify", auditChainHandler(&stubChain{err: errors.New("db")}).Verify)
	resp, err := app.Test(httptest.NewRequest("GET", "/audit/verify", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuditHandlerExport(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	app := newTestApp(tenantID, "admin")
	app.Get("/audit/export", middleware.RequireAdminRole, auditChainHandler(&stubChain{recs: stubChainOf(tenantID, 5)}).Export)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit/export?from_seq=2&limit=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("status=%d ct=%q body=%s", resp.StatusCode, resp.Header.Get("Content-Type"), raw)
	}
	if resp.Header.Get("X-PCMI-Audit-Complete") != "false" || resp.Header.Get("X-PCMI-Audit-Head-Hash") == "" {
		t.Fatalf("headers: %v", resp.Header)
	}
	chk, err := auditchain.VerifyExport(strings.NewReader(string(raw)), "k")
	if err != nil || !chk.Valid || chk.Entries != 2 || chk.Trailer.NextFromSeq != 4 {
		t.Fatalf("chk=%+v err=%v", chk, err)
	}
}

func TestAuditHandlerExport_badParams(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	app := newTestApp(tenantID, "admin")
	app.Get("/audit/export", auditChainHandler(&stubChain{}).Export)
	for _, q := range []string{"from_seq=x", "to_seq=1.5", "limit=abc", "limit=0&from_seq=-3", "limit=999999"} {
		resp, err := app.Test(httptest.NewRequest("GET", "/audit/export?"+q, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("%s: status %d want 400", q, resp.StatusCode)
		}
	}
}

func TestAuditHandlerExport_error(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	app := newTestApp(tenantID, "admin")
	app.Get("/audit/export", auditChainHandler(&stubChain{err: errors.New("db")}).Export)
	resp, err := app.Test(httptest.NewRequest("GET", "/audit/export", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuditHandlerExport_requiresAdmin(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	app := newTestApp(tenantID, "user")
	app.Get("/audit/export", middleware.RequireAdminRole, auditChainHandler(&stubChain{}).Export)
	resp, err := app.Test(httptest.NewRequest("GET", "/audit/export", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status %d want 403", resp.StatusCode)
	}
}
