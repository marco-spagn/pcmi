-- 023_entity_graph.sql — Phase B entity vertices in AGE (optional, like 019).
-- Promotes extracted slots to :Entity vertices with :mentions edges from Memory.
-- Requires AGE; skipped gracefully when the extension is not installed.

DO $outer$
BEGIN

IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'age') THEN
    RAISE NOTICE 'AGE extension not available — skipping entity graph setup';
    RETURN;
END IF;

CREATE EXTENSION IF NOT EXISTS age;

SET search_path = ag_catalog, "$user", public;

-- ─── helper: reconcile_entity_mentions_for_memory ───────────────────────────
-- Replaces all mentions edges for a memory with the current extraction snapshot.
-- p_memory_id: vertex id string, e.g. memory.4242
-- p_mentions: JSON array of {slot, kind, key, confidence}
CREATE OR REPLACE FUNCTION public.reconcile_entity_mentions_for_memory(
    p_memory_id      text,
    p_tenant_id        uuid,
    p_memory_version   int,
    p_mentions         jsonb
) RETURNS void
SET search_path = ag_catalog, "$user", public
LANGUAGE plpgsql AS $fn$
DECLARE
    mention jsonb;
    safe_slot text;
    safe_kind text;
    ent_key text;
    conf float8;
BEGIN
    -- Ensure the Memory vertex exists (same identity as memory_links sync).
    EXECUTE format(
        $cypher_exec$
        SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
            MERGE (m:Memory {id: %L, tenant_id: %L})
        $cypher$) AS (result ag_catalog.agtype)
        $cypher_exec$,
        p_memory_id, p_tenant_id::text
    );

    -- Drop stale mentions for this memory before re-syncing.
    EXECUTE format(
        $cypher_exec$
        SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
            MATCH (m:Memory {id: %L, tenant_id: %L})-[r:mentions]->()
            DELETE r
        $cypher$) AS (result ag_catalog.agtype)
        $cypher_exec$,
        p_memory_id, p_tenant_id::text
    );

    IF p_mentions IS NULL OR jsonb_typeof(p_mentions) <> 'array' THEN
        RETURN;
    END IF;

    FOR mention IN SELECT * FROM jsonb_array_elements(p_mentions)
    LOOP
        safe_slot := regexp_replace(COALESCE(mention->>'slot', ''), '[^\w]', '_', 'g');
        safe_kind := regexp_replace(COALESCE(mention->>'kind', ''), '[^\w]', '_', 'g');
        ent_key   := COALESCE(mention->>'key', '');
        conf      := COALESCE((mention->>'confidence')::float8, 0);

        IF safe_slot = '' OR safe_kind = '' OR ent_key = '' THEN
            CONTINUE;
        END IF;

        EXECUTE format(
            $cypher_exec$
            SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
                MERGE (e:Entity {kind: %L, key: %L, tenant_id: %L})
                WITH e
                MATCH (m:Memory {id: %L, tenant_id: %L})
                MERGE (m)-[r:mentions {slot: %L}]->(e)
                SET r.confidence = %s,
                    r.memory_version = %s,
                    r.kind = %L
            $cypher$) AS (result ag_catalog.agtype)
            $cypher_exec$,
            safe_kind, ent_key, p_tenant_id::text,
            p_memory_id, p_tenant_id::text,
            safe_slot,
            conf,
            p_memory_version,
            safe_kind
        );
    END LOOP;
EXCEPTION
    WHEN others THEN
        RAISE WARNING 'reconcile_entity_mentions_for_memory: %', SQLERRM;
END;
$fn$;

EXCEPTION WHEN undefined_file THEN
        RAISE NOTICE 'AGE extension not available — skipping entity graph setup';
    WHEN SQLSTATE '58P01' THEN
        RAISE NOTICE 'AGE extension not available — skipping entity graph setup';
    WHEN SQLSTATE '0A000' THEN
        RAISE NOTICE 'AGE extension not available — skipping entity graph setup';
    WHEN SQLSTATE '42883' THEN
        RAISE NOTICE 'AGE entity graph helpers not available — skipping entity graph setup';

END;
$outer$;
