-- v1.14.0: consolidation tracking, BM25 rank helper, admin SECURITY DEFINER helpers

CREATE TABLE IF NOT EXISTS consolidation_runs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    path_prefix TEXT NOT NULL,
    source_entry_ids BIGINT[] NOT NULL,
    consolidated_path TEXT NOT NULL,
    consolidated_entry_id BIGINT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_consolidation_runs_tenant ON consolidation_runs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_consolidation_runs_status ON consolidation_runs(status) WHERE status = 'pending';

ALTER TABLE consolidation_runs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_consolidation_runs ON consolidation_runs
    USING (tenant_id = current_setting('app.current_tenant')::uuid);

-- BM25-style rank (ts_rank_cd normalization flag 32)
CREATE OR REPLACE FUNCTION pcmi_bm25_rank(doc_tsv tsvector, q tsquery)
RETURNS float8 AS $$
    SELECT COALESCE(ts_rank_cd(doc_tsv, q, 32), 0)::float8;
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- Platform admin helpers (bypass RLS via SECURITY DEFINER)
CREATE OR REPLACE FUNCTION admin_create_tenant(p_slug TEXT, p_name TEXT, p_settings JSONB DEFAULT '{}')
RETURNS TABLE(id UUID, slug TEXT, name TEXT) AS $$
BEGIN
    RETURN QUERY
    INSERT INTO tenants (slug, name, settings)
    VALUES (p_slug, p_name, COALESCE(p_settings, '{}'::jsonb))
    RETURNING tenants.id, tenants.slug, tenants.name;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

CREATE OR REPLACE FUNCTION admin_list_tenants(p_limit INT DEFAULT 100)
RETURNS TABLE(id UUID, slug TEXT, name TEXT, settings JSONB, created_at TIMESTAMPTZ) AS $$
BEGIN
    RETURN QUERY
    SELECT t.id, t.slug, t.name, t.settings, t.created_at
    FROM tenants t
    ORDER BY t.created_at DESC
    LIMIT LEAST(GREATEST(p_limit, 1), 500);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

CREATE OR REPLACE FUNCTION admin_rotate_api_key(p_key_id UUID, p_new_hash TEXT, p_name TEXT DEFAULT NULL)
RETURNS TABLE(id UUID, tenant_id UUID, name TEXT, role TEXT) AS $$
BEGIN
    RETURN QUERY
    UPDATE api_keys
    SET key_hash = p_new_hash,
        name = COALESCE(p_name, api_keys.name),
        last_used_at = NULL,
        is_active = true
    WHERE api_keys.id = p_key_id
    RETURNING api_keys.id, api_keys.tenant_id, api_keys.name, api_keys.role;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

CREATE OR REPLACE FUNCTION admin_create_api_key(
    p_tenant_id UUID, p_key_hash TEXT, p_name TEXT, p_role TEXT, p_expires_at TIMESTAMPTZ DEFAULT NULL
)
RETURNS TABLE(id UUID, tenant_id UUID, name TEXT, role TEXT) AS $$
BEGIN
    RETURN QUERY
    INSERT INTO api_keys (tenant_id, key_hash, name, role, expires_at, is_active)
    VALUES (p_tenant_id, p_key_hash, p_name, p_role, p_expires_at, true)
    RETURNING api_keys.id, api_keys.tenant_id, api_keys.name, api_keys.role;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
