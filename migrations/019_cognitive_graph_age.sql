-- 019_cognitive_graph_age.sql — v2.0 spike — requires AGE extension.
-- This migration is OPTIONAL and skipped if AGE is not installed.
--
-- Apache AGE (https://github.com/apache/age) adds Cypher graph query support to
-- PostgreSQL.  Install via: docker compose --profile graph up
--
-- All statements are wrapped in a DO block that catches undefined_file (raised
-- when the AGE shared library is missing) and raises a NOTICE instead of failing.

DO $outer$
BEGIN

-- Verify AGE is present before proceeding.
CREATE EXTENSION IF NOT EXISTS age;  -- requires Apache AGE

SET search_path = ag_catalog, "$user", public;

-- Create the cognitive memory graph (idempotent).
PERFORM create_graph('pcmi_memory_graph');

-- Add weight column to memory_links if not already present.
-- Used by the graph sync trigger to record edge strength.
ALTER TABLE public.memory_links
    ADD COLUMN IF NOT EXISTS weight float8 NOT NULL DEFAULT 1.0;

-- ─── helper: sync_memory_link_to_graph ────────────────────────────────────────
-- Merges two Memory vertices and a typed directed edge into the AGE graph.
-- p_from_path / p_to_path: LTREE path strings used as vertex identity keys.
-- p_link_type: becomes the Cypher relationship label (sanitised, alphanumeric+_).
-- p_weight: stored as an edge property.
-- p_tenant_id: scopes vertices so cross-tenant traversals are impossible.
CREATE OR REPLACE FUNCTION public.sync_memory_link_to_graph(
    p_from_path text,
    p_to_path   text,
    p_link_type text,
    p_weight    float8,
    p_tenant_id uuid
) RETURNS void LANGUAGE plpgsql AS $fn$
DECLARE
    safe_type text;
BEGIN
    -- Sanitise link_type: keep only word characters to prevent Cypher injection.
    safe_type := regexp_replace(p_link_type, '[^\w]', '_', 'g');

    -- Dynamic relationship label requires EXECUTE; AGE does not support
    -- variable labels in MERGE.  safe_type is already sanitised above.
    EXECUTE format(
        $cypher_exec$
        SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
            MERGE (a:Memory {id: %L, tenant_id: %L})
            MERGE (b:Memory {id: %L, tenant_id: %L})
            MERGE (a)-[r:%I {weight: %s}]->(b)
        $cypher$) AS (result ag_catalog.agtype)
        $cypher_exec$,
        p_from_path, p_tenant_id::text,
        p_to_path,   p_tenant_id::text,
        safe_type,   p_weight
    );
EXCEPTION
    WHEN others THEN
        -- Graph sync is best-effort; do not fail the originating INSERT.
        RAISE WARNING 'sync_memory_link_to_graph: %', SQLERRM;
END;
$fn$;

-- ─── trigger: trg_memory_links_sync_graph ─────────────────────────────────────
CREATE OR REPLACE FUNCTION public.trg_memory_links_sync_graph_fn()
RETURNS trigger LANGUAGE plpgsql AS $trig$
BEGIN
    PERFORM public.sync_memory_link_to_graph(
        NEW.from_path::text,
        NEW.to_path::text,
        NEW.link_type,
        COALESCE((NEW.metadata->>'weight')::float8, NEW.weight, 1.0),
        NEW.tenant_id
    );
    RETURN NEW;
END;
$trig$;

DROP TRIGGER IF EXISTS trg_memory_links_sync_graph ON public.memory_links;
CREATE TRIGGER trg_memory_links_sync_graph
    AFTER INSERT ON public.memory_links
    FOR EACH ROW EXECUTE FUNCTION public.trg_memory_links_sync_graph_fn();

EXCEPTION WHEN undefined_file THEN
        RAISE NOTICE 'AGE extension not available — skipping cognitive graph setup';
    WHEN SQLSTATE '58P01' THEN
        RAISE NOTICE 'AGE extension not available — skipping cognitive graph setup';
    WHEN SQLSTATE '42883' THEN
        RAISE NOTICE 'AGE create_graph not available — skipping cognitive graph setup';

END;
$outer$;
