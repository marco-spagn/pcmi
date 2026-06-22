//go:build integration

package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

const itestTenant = "00000000-0000-0000-0000-000000000000"

// TestIntegration_ExpiryWorker_closesExpiresAt verifies the expiry worker closes
// rows whose first-class expires_at column is in the past (regression for the bug
// where only metadata.ttl_seconds was honoured and expires_at was ignored).
func TestIntegration_ExpiryWorker_closesExpiresAt(t *testing.T) {
	p := testWorkerPool(t)
	ctx := context.Background()
	path := fmt.Sprintf("root.itest_exp_%d", time.Now().UnixNano())

	if _, err := p.Exec(ctx, "SELECT set_tenant_context($1::uuid)", itestTenant); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, "DELETE FROM memory_entries WHERE path = $1::ltree", path)
	})

	// Stored with an expired expires_at and NO metadata.ttl_seconds.
	_, err := p.Exec(ctx, `
		INSERT INTO memory_entries (tenant_id, path, content, embedding_model, version, valid_from, created_at, expires_at)
		VALUES ($1::uuid, $2::ltree, 'expired', 'unspecified', 1, NOW()-interval '2 hours', NOW()-interval '2 hours', NOW()-interval '1 hour')`,
		itestTenant, path)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	w := &ExpiryWorker{db: p, interval: time.Hour}
	w.runOnce()

	var validToSet bool
	if err := p.QueryRow(ctx,
		"SELECT valid_to IS NOT NULL FROM memory_entries WHERE path = $1::ltree", path,
	).Scan(&validToSet); err != nil {
		t.Fatalf("select valid_to: %v", err)
	}
	if !validToSet {
		t.Fatal("expected memory with past expires_at to be closed (valid_to set), but it is still current")
	}
}

// TestIntegration_ExpiryWorker_toleratesNonIntegerTTL verifies that a current row
// carrying a non-integer metadata.ttl_seconds (free-form client JSONB) does NOT
// abort the whole expiry UPDATE: a sibling row with a valid, past integer TTL must
// still be closed, and the poison row must be left untouched (not crash, not
// expired). Against the pre-fix SQL the (metadata->>'ttl_seconds')::int cast errors
// and zero rows expire for every tenant.
func TestIntegration_ExpiryWorker_toleratesNonIntegerTTL(t *testing.T) {
	p := testWorkerPool(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("root.itest_ttl_%d", time.Now().UnixNano())
	poisonPath := prefix + ".poison"
	validPath := prefix + ".valid"

	if _, err := p.Exec(ctx, "SELECT set_tenant_context($1::uuid)", itestTenant); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, "DELETE FROM memory_entries WHERE path <@ $1::ltree", prefix)
	})

	insert := func(path, ttlJSON string) {
		_, err := p.Exec(ctx, `
			INSERT INTO memory_entries (tenant_id, path, content, embedding_model, version, valid_from, created_at, metadata)
			VALUES ($1::uuid, $2::ltree, 'c', 'unspecified', 1, NOW()-interval '2 hours', NOW()-interval '2 hours', $3::jsonb)`,
			itestTenant, path, ttlJSON)
		if err != nil {
			t.Fatalf("insert %s: %v", path, err)
		}
	}
	// Non-integer ttl_seconds (would crash the cast) and a valid, already-elapsed one.
	insert(poisonPath, `{"ttl_seconds": "not-a-number"}`)
	insert(validPath, `{"ttl_seconds": 3600}`)

	w := &ExpiryWorker{db: p, interval: time.Hour}
	w.runOnce()

	closed := func(path string) bool {
		var set bool
		if err := p.QueryRow(ctx,
			"SELECT valid_to IS NOT NULL FROM memory_entries WHERE path = $1::ltree", path,
		).Scan(&set); err != nil {
			t.Fatalf("select valid_to for %s: %v", path, err)
		}
		return set
	}

	if !closed(validPath) {
		t.Fatal("expected row with valid past ttl_seconds to be closed, but it is still current")
	}
	if closed(poisonPath) {
		t.Fatal("row with non-integer ttl_seconds must be ignored, not closed")
	}
}

// TestIntegration_ContradictionWorker_checkPrefix verifies checkPrefix runs the
// corrected query (column `content`, valid ltree exclusion) without error and
// writes a `contradicts` link for a contradictory pair. Against the previous
// query it errored on the non-existent `text_content` column and created nothing.
func TestIntegration_ContradictionWorker_checkPrefix(t *testing.T) {
	p := testWorkerPool(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("root.itest_con_%d", time.Now().UnixNano())
	pathA := prefix + ".a"
	pathB := prefix + ".b"

	if _, err := p.Exec(ctx, "SELECT set_tenant_context($1::uuid)", itestTenant); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, "DELETE FROM memory_entries WHERE path <@ $1::ltree", prefix)
		_, _ = p.Exec(ctx, "DELETE FROM memory_links WHERE from_path <@ $1::ltree OR to_path <@ $1::ltree", prefix)
	})

	insert := func(path, content string) {
		_, err := p.Exec(ctx, `
			INSERT INTO memory_entries (tenant_id, path, content, embedding_model, version, valid_from, created_at)
			VALUES ($1::uuid, $2::ltree, $3, 'unspecified', 1, NOW(), NOW())`,
			itestTenant, path, content)
		if err != nil {
			t.Fatalf("insert %s: %v", path, err)
		}
	}
	insert(pathA, "the deployment pipeline is fully automated and stable in production environment")
	insert(pathB, "the deployment pipeline is not automated and not stable in production environment")

	w := NewContradictionWorker(p, &config.Config{ContradictionDetectionIntervalSecs: 120})
	if err := w.checkPrefix(ctx, itestTenant, prefix); err != nil {
		t.Fatalf("checkPrefix returned error (query should be valid now): %v", err)
	}

	var links int
	if err := p.QueryRow(ctx, `
		SELECT COUNT(*) FROM memory_links
		WHERE tenant_id = $1::uuid AND link_type = 'contradicts'
		  AND (from_path <@ $2::ltree AND to_path <@ $2::ltree)`,
		itestTenant, prefix,
	).Scan(&links); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if links == 0 {
		t.Fatal("expected a 'contradicts' link to be created for the contradictory pair, got none")
	}
}
