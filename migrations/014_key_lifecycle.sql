-- v1.40 API key rotation grace period, revocation, and usage tracking (PCMI-007)

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS rotated_to UUID REFERENCES api_keys(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS rotation_grace_ends_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_used_ip TEXT;

CREATE INDEX IF NOT EXISTS idx_api_keys_rotated_to ON api_keys(rotated_to);
CREATE INDEX IF NOT EXISTS idx_api_keys_rotation_grace ON api_keys(rotation_grace_ends_at)
    WHERE rotation_grace_ends_at IS NOT NULL;

-- Replace in-place rotate (010) with grace-period rotation (new row + previous hash TTL).
DROP FUNCTION IF EXISTS admin_rotate_api_key(UUID, TEXT, TEXT);

-- Rotate: insert a new key row; old key keeps its hash until grace expires.
CREATE OR REPLACE FUNCTION admin_rotate_api_key(
    p_key_id UUID,
    p_new_hash TEXT,
    p_name TEXT DEFAULT NULL,
    p_grace_interval INTERVAL DEFAULT INTERVAL '24 hours'
)
RETURNS TABLE(id UUID, tenant_id UUID, name TEXT, role TEXT, previous_key_id UUID, grace_ends_at TIMESTAMPTZ) AS $$
DECLARE
    v_old api_keys%ROWTYPE;
    v_new_id UUID;
    v_grace TIMESTAMPTZ;
BEGIN
    SELECT * INTO v_old FROM api_keys WHERE api_keys.id = p_key_id FOR UPDATE;
    IF NOT FOUND THEN
        RETURN;
    END IF;
    IF v_old.rotated_to IS NOT NULL AND v_old.rotation_grace_ends_at IS NOT NULL AND v_old.rotation_grace_ends_at <= NOW() THEN
        RAISE EXCEPTION 'api key already rotated and grace period expired';
    END IF;

    v_grace := NOW() + p_grace_interval;

    INSERT INTO api_keys (tenant_id, key_hash, name, role, expires_at, is_active)
    VALUES (v_old.tenant_id, p_new_hash, COALESCE(p_name, v_old.name), v_old.role, v_old.expires_at, true)
    RETURNING api_keys.id INTO v_new_id;

    UPDATE api_keys
    SET rotated_to = v_new_id,
        rotation_grace_ends_at = v_grace,
        is_active = true
    WHERE api_keys.id = p_key_id;

    RETURN QUERY
    SELECT v_new_id, v_old.tenant_id, COALESCE(p_name, v_old.name), v_old.role, p_key_id, v_grace;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

CREATE OR REPLACE FUNCTION admin_revoke_api_key(p_key_id UUID)
RETURNS TABLE(id UUID, tenant_id UUID, name TEXT, role TEXT) AS $$
BEGIN
    RETURN QUERY
    UPDATE api_keys
    SET is_active = false,
        rotation_grace_ends_at = NULL
    WHERE api_keys.id = p_key_id
      AND api_keys.is_active = true
    RETURNING api_keys.id, api_keys.tenant_id, api_keys.name, api_keys.role;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Record rotation in audit_log (admin operator action).
CREATE OR REPLACE FUNCTION admin_audit_api_key_rotation(
    p_tenant_id UUID,
    p_api_key_id UUID,
    p_previous_key_id UUID,
    p_path TEXT,
    p_method TEXT,
    p_status_code INT,
    p_ip TEXT
) RETURNS VOID AS $$
BEGIN
    INSERT INTO audit_log (
        tenant_id, api_key_id, event_type, path, method, status_code,
        request_body, response_body, ip_address, user_agent, created_at
    ) VALUES (
        p_tenant_id,
        p_api_key_id,
        'api_key_rotation',
        p_path,
        p_method,
        p_status_code,
        jsonb_build_object('previous_key_id', p_previous_key_id::text),
        NULL,
        NULLIF(p_ip, '')::inet,
        'pcmi-admin',
        NOW()
    );
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
