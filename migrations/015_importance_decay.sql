-- v1.42.0: memory importance scoring, access tracking, temporal decay config (PCMI-009)

ALTER TABLE memory_entries
    ADD COLUMN IF NOT EXISTS importance FLOAT NOT NULL DEFAULT 0.5
        CHECK (importance >= 0 AND importance <= 1),
    ADD COLUMN IF NOT EXISTS access_count INTEGER NOT NULL DEFAULT 0
        CHECK (access_count >= 0),
    ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_memories_importance
    ON memory_entries (tenant_id, importance DESC)
    WHERE valid_to IS NULL;

CREATE TABLE IF NOT EXISTS tenant_memory_config (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    weight_semantic FLOAT NOT NULL DEFAULT 0.40 CHECK (weight_semantic >= 0),
    weight_lexical FLOAT NOT NULL DEFAULT 0.30 CHECK (weight_lexical >= 0),
    weight_importance FLOAT NOT NULL DEFAULT 0.15 CHECK (weight_importance >= 0),
    weight_recency FLOAT NOT NULL DEFAULT 0.15 CHECK (weight_recency >= 0),
    decay_halflife_days FLOAT NOT NULL DEFAULT 30.0 CHECK (decay_halflife_days > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tenant_memory_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_memory_config_isolation ON tenant_memory_config
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
