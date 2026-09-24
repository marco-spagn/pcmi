-- v1.52 Tamper-evident audit log (per-tenant SHA-256 hash chain)
-- =============================================================
--
-- Every audit_log row gets a per-tenant sequence number (chain_seq), the hash
-- of the previous row in that tenant's chain (prev_hash), and its own hash:
--
--   row_hash = hex(sha256(prev_hash || '\n' || audit_log_canonical(row)))
--
-- The first row of a tenant chain uses 64 '0' characters as prev_hash. Any
-- edit, deletion, or reordering of a historical row changes a recomputed
-- row_hash and breaks the chain, which GET /v1/audit/verify and the offline
-- verifier of GET /v1/audit/export detect.
--
-- audit_log becomes append-only: UPDATE and DELETE are rejected unless the
-- session sets `pcmi.audit_maintenance = 'on'` (documented operator escape
-- hatch; its use is itself detectable because it breaks the chain).
--
-- Idempotent: safe to re-apply.

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS chain_seq BIGINT;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS prev_hash TEXT;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS row_hash  TEXT;

-- 1. Canonical text form of a row. Every field that the chain protects is
--    included, NULLs are encoded as '' and fields are separated by U+001F so
--    a value can never shift into its neighbour. The leading 'v1' versions
--    the format.
CREATE OR REPLACE FUNCTION audit_log_canonical(
    p_tenant_id     UUID,
    p_chain_seq     BIGINT,
    p_api_key_id    UUID,
    p_event_type    TEXT,
    p_path          TEXT,
    p_method        TEXT,
    p_status_code   INTEGER,
    p_request_body  JSONB,
    p_response_body JSONB,
    p_ip_address    INET,
    p_user_agent    TEXT,
    p_created_at    TIMESTAMPTZ
) RETURNS TEXT AS $$
    SELECT concat_ws(E'\x1f',
        'v1',
        p_tenant_id::text,
        p_chain_seq::text,
        COALESCE(p_api_key_id::text, ''),
        COALESCE(p_event_type, ''),
        COALESCE(p_path, ''),
        COALESCE(p_method, ''),
        COALESCE(p_status_code::text, ''),
        COALESCE(p_request_body::text, ''),
        COALESCE(p_response_body::text, ''),
        COALESCE(host(p_ip_address), ''),
        COALESCE(p_user_agent, ''),
        COALESCE(to_char(p_created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '')
    );
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION audit_log_hash(p_prev_hash TEXT, p_canonical TEXT)
RETURNS TEXT AS $$
    SELECT encode(sha256(convert_to(p_prev_hash || E'\n' || p_canonical, 'UTF8')), 'hex');
$$ LANGUAGE sql IMMUTABLE;

-- 2. Backfill rows written before this migration, per tenant, in insertion
--    order. Continues an existing chain when re-applied.
DO $$
DECLARE
    r        RECORD;
    v_tenant UUID := NULL;
    v_seq    BIGINT;
    v_prev   TEXT;
BEGIN
    -- Allow the backfill UPDATEs through the append-only guard on re-apply.
    PERFORM set_config('pcmi.audit_maintenance', 'on', true);
    FOR r IN
        SELECT id, tenant_id, api_key_id, event_type, path, method, status_code,
               request_body, response_body, ip_address, user_agent, created_at
        FROM audit_log
        WHERE row_hash IS NULL
        ORDER BY tenant_id, created_at, id
    LOOP
        IF v_tenant IS DISTINCT FROM r.tenant_id THEN
            v_tenant := r.tenant_id;
            SELECT chain_seq, row_hash INTO v_seq, v_prev
            FROM audit_log
            WHERE tenant_id = r.tenant_id AND row_hash IS NOT NULL
            ORDER BY chain_seq DESC
            LIMIT 1;
            v_seq  := COALESCE(v_seq, 0);
            v_prev := COALESCE(v_prev, repeat('0', 64));
        END IF;
        v_seq := v_seq + 1;
        UPDATE audit_log
        SET chain_seq = v_seq,
            prev_hash = v_prev,
            row_hash  = audit_log_hash(v_prev, audit_log_canonical(
                r.tenant_id, v_seq, r.api_key_id, r.event_type, r.path, r.method,
                r.status_code, r.request_body, r.response_body, r.ip_address,
                r.user_agent, r.created_at))
        WHERE id = r.id
        RETURNING row_hash INTO v_prev;
    END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_audit_log_tenant_chain_seq
    ON audit_log (tenant_id, chain_seq);

-- 3. Chain every new row. A per-tenant transaction-scoped advisory lock
--    serialises concurrent inserts so the chain never forks; the unique
--    index above is the backstop. SECURITY DEFINER so the tail lookup sees
--    the tenant's chain even when RLS is enforced for the calling role.
CREATE OR REPLACE FUNCTION audit_log_chain_before_insert() RETURNS TRIGGER AS $$
DECLARE
    v_seq  BIGINT;
    v_prev TEXT;
BEGIN
    -- Restore path (pg_restore / COPY of rows that already carry a chain):
    -- keep them verbatim. Verification recomputes every hash, so a forged
    -- row inserted this way is still detected.
    IF NEW.row_hash IS NOT NULL AND NEW.chain_seq IS NOT NULL AND NEW.prev_hash IS NOT NULL THEN
        RETURN NEW;
    END IF;

    PERFORM pg_advisory_xact_lock(hashtextextended('pcmi.audit_chain:' || NEW.tenant_id::text, 0));

    IF NEW.created_at IS NULL THEN
        NEW.created_at := NOW();
    END IF;

    SELECT chain_seq, row_hash INTO v_seq, v_prev
    FROM audit_log
    WHERE tenant_id = NEW.tenant_id AND chain_seq IS NOT NULL
    ORDER BY chain_seq DESC
    LIMIT 1;

    NEW.chain_seq := COALESCE(v_seq, 0) + 1;
    NEW.prev_hash := COALESCE(v_prev, repeat('0', 64));
    NEW.row_hash  := audit_log_hash(NEW.prev_hash, audit_log_canonical(
        NEW.tenant_id, NEW.chain_seq, NEW.api_key_id, NEW.event_type, NEW.path,
        NEW.method, NEW.status_code, NEW.request_body, NEW.response_body,
        NEW.ip_address, NEW.user_agent, NEW.created_at));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;

DROP TRIGGER IF EXISTS trg_audit_log_chain ON audit_log;
CREATE TRIGGER trg_audit_log_chain
    BEFORE INSERT ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_chain_before_insert();

-- 4. Append-only guard.
CREATE OR REPLACE FUNCTION audit_log_block_mutation() RETURNS TRIGGER AS $$
BEGIN
    IF current_setting('pcmi.audit_maintenance', true) = 'on' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'audit_log is append-only (% rejected)', TG_OP
        USING ERRCODE = 'insufficient_privilege',
              HINT = 'Set pcmi.audit_maintenance = on for a documented operator procedure; doing so breaks the hash chain.';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_audit_log_append_only ON audit_log;
CREATE TRIGGER trg_audit_log_append_only
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_block_mutation();
