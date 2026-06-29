//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/marco-spagn/pcmi/internal/model"
)

// link_type_check is the CHECK constraint added in migration 021. These tests
// exercise the database-level backstop directly with raw INSERT/UPDATE
// (bypassing model.NormalizeLinkType, which the repository write path applies),
// because the constraint exists to protect every OTHER writer — the dedup
// engine, manual SQL, future call sites — not just the normalized public path.
const linkTypeCheckConstraint = "memory_links_link_type_check"

// insertLinkRaw inserts straight into memory_links with no Go-side validation,
// returning the database error (if any).
func insertLinkRaw(ctx context.Context, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, tenantID, from, to, linkType string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type)
		VALUES ($1::uuid, $2::ltree, $3::ltree, $4)`,
		tenantID, from, to, linkType)
	return err
}

// assertCheckViolation fails the test unless err is a Postgres check_violation
// (SQLSTATE 23514) raised by the link_type CHECK constraint.
func assertCheckViolation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected check_violation, got nil error")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23514" {
		t.Fatalf("expected SQLSTATE 23514 (check_violation), got %s: %v", pgErr.Code, pgErr)
	}
	if pgErr.ConstraintName != linkTypeCheckConstraint {
		t.Fatalf("expected constraint %q, got %q", linkTypeCheckConstraint, pgErr.ConstraintName)
	}
}

func TestIntegration_LinkTypeCheckConstraint(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	tenantID := "00000000-0000-0000-0000-000000000000"
	setTenant(t, ctx, pool, tenantID)

	base := fmt.Sprintf("root.linkcheck.%d", time.Now().UnixNano())
	t.Cleanup(func() {
		// Best-effort teardown so reruns don't trip the unique index.
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM memory_links WHERE tenant_id = $1::uuid AND from_path <@ $2::ltree`,
			tenantID, base)
	})

	t.Run("accepts_all_valid_types", func(t *testing.T) {
		// Every type the constraint must allow: the five user-assignable types
		// plus the dedup engine's reserved "duplicate".
		valid := append(append([]string{}, model.PublicLinkTypes...), model.DedupLinkType())
		for i, lt := range valid {
			from := fmt.Sprintf("%s.ok%d.a", base, i)
			to := fmt.Sprintf("%s.ok%d.b", base, i)
			if err := insertLinkRaw(ctx, pool, tenantID, from, to, lt); err != nil {
				t.Fatalf("valid link_type %q should be accepted: %v", lt, err)
			}
		}
	})

	t.Run("rejects_unknown_type", func(t *testing.T) {
		err := insertLinkRaw(ctx, pool, tenantID, base+".bad.a", base+".bad.b", "sibling")
		assertCheckViolation(t, err)
	})

	t.Run("rejects_empty_type", func(t *testing.T) {
		// The DB column has no default; normalization to "related" lives in the
		// app, so a writer that skips it and passes "" must be rejected here.
		err := insertLinkRaw(ctx, pool, tenantID, base+".empty.a", base+".empty.b", "")
		assertCheckViolation(t, err)
	})

	t.Run("rejects_update_to_unknown_type", func(t *testing.T) {
		from, to := base+".upd.a", base+".upd.b"
		if err := insertLinkRaw(ctx, pool, tenantID, from, to, "related"); err != nil {
			t.Fatalf("seed valid link: %v", err)
		}
		_, err := pool.Exec(ctx, `
			UPDATE memory_links SET link_type = 'nonsense'
			WHERE tenant_id = $1::uuid AND from_path = $2::ltree AND to_path = $3::ltree`,
			tenantID, from, to)
		assertCheckViolation(t, err)
	})
}
