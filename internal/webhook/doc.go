// Package webhook gestisce la consegna HTTP verso URL registrati dal tenant: match su tipo evento,
// firma opzionale, retry con backoff e registrazione dead-letter per diagnostica.
package webhook
