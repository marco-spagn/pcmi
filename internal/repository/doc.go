// Package repository implementa l’accesso a PostgreSQL con pgxpool. Ogni query operativa
// assume che il tenant corrente sia già stato impostato (middleware chiama set_tenant_context).
// Le funzioni di retrieve condividono helper SQL in retrieve_sql.go per filtri temporali,
// scope agent/embedding space e tag. La persistenza è append-only per le versioni di memoria.
package repository
