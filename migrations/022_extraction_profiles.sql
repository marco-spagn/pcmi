-- v2.0 Phase A: tenant extraction profiles for LLM attribute slot filling.

CREATE TABLE IF NOT EXISTS extraction_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL,
    version INT NOT NULL DEFAULT 1 CHECK (version >= 1),
    path_prefix LTREE NOT NULL DEFAULT 'root',
    profile JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, profile_id)
);

CREATE INDEX IF NOT EXISTS idx_extraction_profiles_tenant_enabled
    ON extraction_profiles (tenant_id)
    WHERE enabled = true;

CREATE INDEX IF NOT EXISTS idx_extraction_profiles_path_prefix
    ON extraction_profiles USING gist (path_prefix);

ALTER TABLE extraction_profiles ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_extraction_profiles ON extraction_profiles
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
