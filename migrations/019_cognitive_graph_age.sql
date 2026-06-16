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

-- Skip cleanly when AGE is not installed (e.g. CI pgvector/pg16 service).
IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'age') THEN
    RAISE NOTICE 'AGE extension not available — skipping cognitive graph setup';
    RETURN;
END IF;

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
) RETURNS void
SET search_path = ag_catalog, "$user", public
LANGUAGE plpgsql AS $fn$
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

-- ─── helper: delete_memory_link_from_graph ────────────────────────────────────
-- Removes the corresponding AGE relationship when the SQL memory_links row is
-- deleted or re-keyed.  Best-effort, same as sync_memory_link_to_graph.
CREATE OR REPLACE FUNCTION public.delete_memory_link_from_graph(
    p_from_path text,
    p_to_path   text,
    p_link_type text,
    p_tenant_id uuid
) RETURNS void
SET search_path = ag_catalog, "$user", public
LANGUAGE plpgsql AS $fn$
DECLARE
    safe_type text;
BEGIN
    safe_type := regexp_replace(p_link_type, '[^\w]', '_', 'g');

    EXECUTE format(
        $cypher_exec$
        SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
            MATCH (a:Memory {id: %L, tenant_id: %L})-[r:%I]->(b:Memory {id: %L, tenant_id: %L})
            DELETE r
        $cypher$) AS (result ag_catalog.agtype)
        $cypher_exec$,
        p_from_path, p_tenant_id::text,
        safe_type,
        p_to_path,   p_tenant_id::text
    );
EXCEPTION
    WHEN others THEN
        RAISE WARNING 'delete_memory_link_from_graph: %', SQLERRM;
END;
$fn$;

-- ─── trigger: trg_memory_links_sync_graph ─────────────────────────────────────
CREATE OR REPLACE FUNCTION public.trg_memory_links_sync_graph_fn()
RETURNS trigger LANGUAGE plpgsql AS $trig$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM public.delete_memory_link_from_graph(
            OLD.from_path::text,
            OLD.to_path::text,
            OLD.link_type,
            OLD.tenant_id
        );
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' AND (
        OLD.from_path IS DISTINCT FROM NEW.from_path OR
        OLD.to_path IS DISTINCT FROM NEW.to_path OR
        OLD.link_type IS DISTINCT FROM NEW.link_type OR
        OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
    ) THEN
        PERFORM public.delete_memory_link_from_graph(
            OLD.from_path::text,
            OLD.to_path::text,
            OLD.link_type,
            OLD.tenant_id
        );
    END IF;

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
    AFTER INSERT OR UPDATE OR DELETE ON public.memory_links
    FOR EACH ROW EXECUTE FUNCTION public.trg_memory_links_sync_graph_fn();

EXCEPTION WHEN undefined_file THEN
        RAISE NOTICE 'AGE extension not available — skipping cognitive graph setup';
    WHEN SQLSTATE '58P01' THEN
        RAISE NOTICE 'AGE extension not available — skipping cognitive graph setup';
    WHEN SQLSTATE '0A000' THEN
        RAISE NOTICE 'AGE extension not available — skipping cognitive graph setup';
    WHEN SQLSTATE '42883' THEN
        RAISE NOTICE 'AGE create_graph not available — skipping cognitive graph setup';

END;
$outer$;
