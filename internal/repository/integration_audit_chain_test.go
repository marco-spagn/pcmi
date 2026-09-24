//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/auditchain"
)

func createChainTenant(t *testing.T, ctx context.Context, repo *AuditRepository) string {
	t.Helper()
	var id string
	slug := fmt.Sprintf("audit-chain-%d", time.Now().UnixNano())
	if err := repo.db.QueryRow(ctx, `INSERT INTO tenants (name, slug) VALUES ($1, $1) RETURNING id::text`, slug).Scan(&id); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return id
}

// TestIntegration_AuditChain_goMatchesSQL asserts the Go canonical form and
// hash reproduce what the migration-027 trigger stored, on awkward inputs.
func TestIntegration_AuditChain_goMatchesSQL(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	repo := NewAuditRepository(pool)
	tenantID := createChainTenant(t, ctx, repo)

	inserts := []struct {
		req, resp, ip, ua any
		path              string
	}{
		{nil, nil, nil, nil, "/v1/plain"},
		{`{"b": [1, 2.50, true, null], "a": "vàlue ✓ <&> \"q\""}`, `{"ok": true}`, "10.1.2.3", "curl/8", ""},
		{`{"nested": {"z": 1, "a": {"k": "éx"}}}`, nil, "2001:db8::1", "ua\twith\ttabs", "/v1/ipv6"},
		{`[]`, `"str"`, "192.168.0.1/32", "", "/v1/😀/unicode"},
	}
	for i, in := range inserts {
		path := in.path
		if path == "" {
			path = fmt.Sprintf("/v1/row%d", i)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO audit_log (tenant_id, event_type, path, method, status_code, request_body, response_body, ip_address, user_agent)
			VALUES ($1::uuid, 'api_request', $2, 'POST', 201, $3::jsonb, $4::jsonb, $5::inet, $6)`,
			tenantID, path, in.req, in.resp, in.ip, in.ua); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	recs, err := repo.ChainBatch(ctx, tenantID, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != len(inserts) {
		t.Fatalf("got %d records", len(recs))
	}
	v := auditchain.NewVerifier(tenantID)
	for _, r := range recs {
		if !v.Add(r) {
			t.Fatalf("Go verification diverges from SQL at seq %d: %+v\nrecord=%+v", r.ChainSeq, v.FirstBreak, r)
		}
	}
	seq, head, err := repo.ChainHead(ctx, tenantID)
	if err != nil || seq != int64(len(inserts)) || head != recs[len(recs)-1].RowHash {
		t.Fatalf("head seq=%d head=%s err=%v", seq, head, err)
	}

	// Append-only guard.
	if _, err := pool.Exec(ctx, `UPDATE audit_log SET path = 'x' WHERE tenant_id = $1::uuid`, tenantID); err == nil {
		t.Fatal("UPDATE on audit_log must be rejected")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1::uuid`, tenantID); err == nil {
		t.Fatal("DELETE on audit_log must be rejected")
	}

	// Tamper through the maintenance escape hatch: verification must catch it.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL pcmi.audit_maintenance = 'on'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE audit_log SET status_code = 200 WHERE tenant_id = $1::uuid AND chain_seq = 2`, tenantID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	recs, err = repo.ChainBatch(ctx, tenantID, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	v = auditchain.NewVerifier(tenantID)
	for _, r := range recs {
		v.Add(r)
	}
	if v.Valid() || v.FirstBreak.ChainSeq != 2 || v.FirstBreak.Reason != auditchain.ReasonHashMismatch {
		t.Fatalf("tamper not detected: %+v", v.FirstBreak)
	}
}

// TestIntegration_AuditChain_concurrentInsertsNeverFork hammers one tenant.
func TestIntegration_AuditChain_concurrentInsertsNeverFork(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	repo := NewAuditRepository(pool)
	tenantID := createChainTenant(t, ctx, repo)

	const n = 50
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			_, err := pool.Exec(ctx, `INSERT INTO audit_log (tenant_id, event_type, path, method, status_code)
				VALUES ($1::uuid, 'api_request', $2, 'GET', 200)`, tenantID, fmt.Sprintf("/c/%d", i))
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	recs, err := repo.ChainBatch(ctx, tenantID, 0, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	v := auditchain.NewVerifier(tenantID)
	for _, r := range recs {
		v.Add(r)
	}
	if !v.Valid() || v.Checked != n || v.HeadSeq() != n {
		t.Fatalf("chain forked or has gaps: checked=%d head=%d break=%+v", v.Checked, v.HeadSeq(), v.FirstBreak)
	}
}
