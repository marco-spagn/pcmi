// pcmi-admin: operator CLI for tenants and API keys (no HTTP server).
// Usage: DATABASE_URL=... go run ./cmd/pcmi-admin list
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/database"
	"github.com/marco-spagn/pcmi/internal/repository"
)

func main() {
	cmd := "list"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "list":
		if err := runList(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "usage: pcmi-admin list [--tenant SLUG|UUID] [--tenant-limit N] [--key-limit N]\n")
		os.Exit(2)
	}
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	tenantFilter := fs.String("tenant", "", "filter by tenant slug or UUID")
	tenantLimit := fs.Int("tenant-limit", 100, "max tenants to scan")
	keyLimit := fs.Int("key-limit", 50, "max API keys per tenant")
	_ = fs.Parse(args)

	dbURL := strings.TrimSpace(config.Load().DatabaseURL)
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required (e.g. postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable)")
	}

	ctx := context.Background()
	pool := database.New(dbURL)
	defer pool.Close()

	repo := repository.NewAdminRepository(pool)

	tenants, err := repo.ListTenants(ctx, *tenantLimit)
	if err != nil {
		return err
	}
	if *tenantFilter != "" {
		n := 0
		for _, t := range tenants {
			if *tenantFilter == t.Slug || *tenantFilter == t.ID {
				tenants[n] = t
				n++
			}
		}
		tenants = tenants[:n]
		if len(tenants) == 0 {
			return fmt.Errorf("no tenant matching %q", *tenantFilter)
		}
	}

	fmt.Printf("=== Tenants (%d) ===\n", len(tenants))
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TENANT_ID\tSLUG\tNAME\tCREATED")
	for _, t := range tenants {
		created := "—"
		if !t.CreatedAt.IsZero() {
			created = t.CreatedAt.UTC().Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.ID, t.Slug, t.Name, created)
	}
	_ = tw.Flush()

	keys, err := repo.ListAllAPIKeysOverview(ctx, *tenantLimit, *keyLimit, *tenantFilter)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== API keys (%d) ===\n", len(keys))
	fmt.Println("(hash_prefix = first 8 chars of SHA-256 stored hash; raw secrets are never shown)")
	tw = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TENANT_SLUG\tKEY_ID\tNAME\tROLE\tACTIVE\tHASH_PREFIX\tCREATED\tEXPIRES\tLAST_USED")
	for _, k := range keys {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\n",
			k.TenantSlug, k.ID, k.Name, k.Role, k.IsActive, k.HashPrefix,
			formatTime(k.CreatedAt), formatOptionalTime(k.ExpiresAt), formatOptionalTime(k.LastUsedAt),
		)
	}
	_ = tw.Flush()
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "—"
	}
	return t.UTC().Format(time.RFC3339)
}
