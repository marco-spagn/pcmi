// Package middleware applica autenticazione API key (hash SHA-256 + tenant RLS), rate limiting
// per chiave, ruoli read/write/admin, e audit delle richieste. Health e metrics sono esentiti
// dove la sicurezza lo consente; vedi singoli file per eccezioni.
package middleware
