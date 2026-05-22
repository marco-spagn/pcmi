-- v1.38.0: idempotency cache for POST /v1/memories (24h TTL)

CREATE TABLE IF NOT EXISTS idempotency_cache (
    idempotency_key TEXT NOT NULL,
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    response_json   JSONB NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_cache_expires_at
    ON idempotency_cache (expires_at);

ALTER TABLE idempotency_cache ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS idempotency_cache_tenant_isolation ON idempotency_cache;
CREATE POLICY idempotency_cache_tenant_isolation ON idempotency_cache
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
