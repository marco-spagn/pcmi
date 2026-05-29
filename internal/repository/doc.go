// Package repository implements PostgreSQL access with pgxpool. Every operational query
// assumes the current tenant has already been set (middleware calls set_tenant_context).
// Retrieve functions share SQL helpers in retrieve_sql.go for temporal filters,
// agent/embedding space scope, and tags. Persistence is append-only for memory versions.
package repository
