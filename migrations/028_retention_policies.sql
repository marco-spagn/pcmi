-- v1.52 Per-tenant / per-namespace data retention + GDPR erasure helpers
-- =====================================================================
--
-- retention_policies attaches retention rules to an ltree path prefix of one
-- tenant. For every memory row the MOST SPECIFIC matching prefix (highest
-- nlevel) that defines a given rule wins; rules are resolved independently,
-- so a child policy that sets only max_age_days still inherits
-- superseded_retention_days from its parent.
--
--   superseded_retention_days  hard-delete closed (superseded/expired) versions
--                              whose valid_to is older than N days. Overrides
--                              the global PRUNE_RETENTION_DAYS for the prefix,
--                              in either direction (keep longer or shorter).
--   max_age_days               soft-close (valid_to = NOW()) CURRENT memories
--                              whose valid_from is older than N days; they are
--                              then purged by the superseded rule above.
--
-- Idempotent: safe to re-apply.

CREATE TABLE IF NOT EXISTS retention_policies (
    id                        BIGSERIAL PRIMARY KEY,
    tenant_id                 UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    path_prefix               LTREE NOT NULL,
    superseded_retention_days INTEGER CHECK (superseded_retention_days IS NULL OR superseded_retention_days BETWEEN 1 AND 36500),
    max_age_days              INTEGER CHECK (max_age_days IS NULL OR max_age_days BETWEEN 1 AND 36500),
    description               TEXT NOT NULL DEFAULT '',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT retention_policies_has_rule CHECK (superseded_retention_days IS NOT NULL OR max_age_days IS NOT NULL),
    UNIQUE (tenant_id, path_prefix)
);

CREATE INDEX IF NOT EXISTS idx_retention_policies_prefix_gist
    ON retention_policies USING GIST (path_prefix);

ALTER TABLE retention_policies ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS retention_policies_tenant_isolation ON retention_policies;
CREATE POLICY retention_policies_tenant_isolation ON retention_policies
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Policy-aware replacement for prune_superseded_memories (migration 008), used
-- by the pruning worker. Rows outside any policy keep the global default.
CREATE OR REPLACE FUNCTION prune_superseded_memories_with_policies(p_default_days int DEFAULT 30)
RETURNS int AS $$
DECLARE
    v_min_days      int;
    v_deleted_count int;
BEGIN
    IF p_default_days IS NULL OR p_default_days < 1 THEN
        RAISE EXCEPTION 'p_default_days must be >= 1 (got %)', p_default_days;
    END IF;

    -- Cheap pre-filter: nothing younger than the shortest effective window can
    -- qualify, whichever policy applies.
    SELECT LEAST(p_default_days, COALESCE(MIN(superseded_retention_days), p_default_days))
    INTO v_min_days
    FROM retention_policies;

    DELETE FROM memory_entries m
    WHERE m.valid_to IS NOT NULL
      AND m.valid_to < NOW() - make_interval(days => v_min_days)
      AND m.valid_to < NOW() - make_interval(days => COALESCE((
            SELECT p.superseded_retention_days
            FROM retention_policies p
            WHERE p.tenant_id = m.tenant_id
              AND p.superseded_retention_days IS NOT NULL
              AND m.path <@ p.path_prefix
            ORDER BY nlevel(p.path_prefix) DESC
            LIMIT 1
          ), p_default_days));
    GET DIAGNOSTICS v_deleted_count = ROW_COUNT;
    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;

-- Soft-close current memories older than their namespace's max_age_days.
-- Append-only like TTL expiry: the row stays readable through as_of until the
-- superseded rule purges it.
CREATE OR REPLACE FUNCTION expire_memories_by_retention_policy()
RETURNS int AS $$
DECLARE
    v_closed int;
BEGIN
    UPDATE memory_entries m
    SET valid_to = NOW()
    WHERE m.valid_to IS NULL
      AND EXISTS (SELECT 1 FROM retention_policies p0
                  WHERE p0.tenant_id = m.tenant_id AND p0.max_age_days IS NOT NULL)
      AND m.valid_from < NOW() - make_interval(days => (
            SELECT p.max_age_days
            FROM retention_policies p
            WHERE p.tenant_id = m.tenant_id
              AND p.max_age_days IS NOT NULL
              AND m.path <@ p.path_prefix
            ORDER BY nlevel(p.path_prefix) DESC
            LIMIT 1
          ));
    GET DIAGNOSTICS v_closed = ROW_COUNT;
    RETURN v_closed;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;

-- Optional (Apache AGE): remove :Memory vertices (and their edges) for erased
-- memories. Vertex ids are either the ltree path (memory_links sync, 019) or
-- 'memory.<id>' (entity mentions, 023). Skipped when AGE is not installed; the
-- application calls it only when the function exists.
DO $outer$
BEGIN
IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'age') THEN
    RAISE NOTICE 'AGE extension not available — skipping erase_memory_graph_vertices';
    RETURN;
END IF;

CREATE EXTENSION IF NOT EXISTS age;

CREATE OR REPLACE FUNCTION public.erase_memory_graph_vertices(
    p_tenant_id  uuid,
    p_vertex_ids text[]
) RETURNS int
SET search_path = ag_catalog, "$user", public
LANGUAGE plpgsql AS $fn$
DECLARE
    v_id    text;
    v_count int := 0;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'pcmi_memory_graph') THEN
        RETURN 0;
    END IF;
    FOREACH v_id IN ARRAY COALESCE(p_vertex_ids, ARRAY[]::text[]) LOOP
        EXECUTE format(
            $cypher_exec$
            SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
                MATCH (m:Memory {id: %L, tenant_id: %L})
                DETACH DELETE m
            $cypher$) AS (result ag_catalog.agtype)
            $cypher_exec$,
            v_id, p_tenant_id::text
        );
        v_count := v_count + 1;
    END LOOP;
    RETURN v_count;
END;
$fn$;
END
$outer$;
