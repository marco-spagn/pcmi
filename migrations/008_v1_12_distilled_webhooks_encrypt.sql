-- v1.12.0: distilled versioning, webhook endpoints, encrypted content flag

ALTER TABLE distilled_knowledge
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;

-- Backfill version numbers for existing rows at the same path
WITH ranked AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY tenant_id, path ORDER BY distilled_at, id) AS rn
    FROM distilled_knowledge
)
UPDATE distilled_knowledge dk
SET version = ranked.rn
FROM ranked
WHERE dk.id = ranked.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_distilled_tenant_path_version
    ON distilled_knowledge (tenant_id, path, version);

ALTER TABLE memory_entries
    ADD COLUMN IF NOT EXISTS content_encrypted BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    event_types TEXT[] NOT NULL DEFAULT '{}',
    secret TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_tenant
    ON webhook_endpoints (tenant_id) WHERE enabled;

ALTER TABLE webhook_endpoints ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_webhook_endpoints ON webhook_endpoints
    USING (tenant_id = current_setting('app.current_tenant')::uuid);

-- Prune superseded memory rows older than retention (worker uses SECURITY DEFINER)
CREATE OR REPLACE FUNCTION prune_superseded_memories(retention_days int DEFAULT 30)
RETURNS int AS $$
DECLARE
    deleted_count int;
BEGIN
    DELETE FROM memory_entries
    WHERE valid_to IS NOT NULL
      AND valid_to < NOW() - (retention_days || ' days')::interval;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;

-- Mark memories for re-embedding (embedding migration)
CREATE OR REPLACE FUNCTION mark_embeddings_for_migration(
    p_tenant_id uuid,
    p_path_prefix text,
    p_target_model text,
    p_embedding_space text DEFAULT NULL
)
RETURNS int AS $$
DECLARE
    updated_count int;
BEGIN
    UPDATE memory_entries
    SET embedding = NULL,
        embedding_model = COALESCE(NULLIF(trim(p_target_model), ''), embedding_model)
    WHERE tenant_id = p_tenant_id
      AND path <@ p_path_prefix::ltree
      AND valid_to IS NULL
      AND (p_embedding_space IS NULL OR embedding_space = p_embedding_space);
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;
