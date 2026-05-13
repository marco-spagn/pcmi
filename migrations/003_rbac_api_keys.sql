-- v1.5 RBAC + API Keys
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user', 'readonly')),
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    is_active BOOLEAN DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_id ON api_keys(tenant_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_expires_at ON api_keys(expires_at);

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;

CREATE POLICY IF NOT EXISTS tenant_api_keys ON api_keys
    USING (tenant_id = current_setting('app.current_tenant')::uuid);

-- Chiave di test (testkey123)
INSERT INTO api_keys (tenant_id, key_hash, name, role, expires_at, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000000',
    '87d452521c9a7f5c9052ae6190e900a46e2a2df5f144158c2fc20b797adb470b',
    'Default Test Key',
    'admin',
    NULL,
    true
) ON CONFLICT (key_hash) DO NOTHING;
