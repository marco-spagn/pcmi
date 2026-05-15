-- v1.15.0: memory links, TTL expiry, tag index

ALTER TABLE memory_entries
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_memories_tags_gin
    ON memory_entries USING GIN (tags);

CREATE INDEX IF NOT EXISTS idx_memories_expires_at
    ON memory_entries (tenant_id, expires_at)
    WHERE expires_at IS NOT NULL AND valid_to IS NULL;

CREATE TABLE IF NOT EXISTS memory_links (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    from_path   LTREE NOT NULL,
    to_path     LTREE NOT NULL,
    link_type   TEXT NOT NULL DEFAULT 'related',
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, from_path, to_path, link_type)
);

CREATE INDEX IF NOT EXISTS idx_memory_links_from
    ON memory_links (tenant_id, from_path);

CREATE INDEX IF NOT EXISTS idx_memory_links_to
    ON memory_links (tenant_id, to_path);

ALTER TABLE memory_links ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS memory_links_tenant_isolation ON memory_links;
CREATE POLICY memory_links_tenant_isolation ON memory_links
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Close memories whose TTL has passed (append-only: set valid_to, do not delete).
CREATE OR REPLACE FUNCTION expire_memory_entries()
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
    n INTEGER;
BEGIN
    UPDATE memory_entries
    SET valid_to = NOW()
    WHERE valid_to IS NULL
      AND expires_at IS NOT NULL
      AND expires_at <= NOW();
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END;
$$;
