-- v2.0 Phase D: AGE helpers for entity alias merge (same_as) and expanded traversal keys.

DO $outer$
BEGIN

IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'age') THEN
    RAISE NOTICE 'AGE extension not available — skipping entity same_as graph setup';
    RETURN;
END IF;

CREATE EXTENSION IF NOT EXISTS age;

SET search_path = ag_catalog, "$user", public;

-- Rewire mentions from an alias entity vertex to a canonical entity vertex, then link same_as.
CREATE OR REPLACE FUNCTION public.merge_entity_alias_in_graph(
    p_tenant_id      uuid,
    p_kind           text,
    p_alias_key      text,
    p_canonical_key  text
) RETURNS void
SET search_path = ag_catalog, "$user", public
LANGUAGE plpgsql AS $fn$
DECLARE
    safe_kind text;
BEGIN
    safe_kind := regexp_replace(COALESCE(p_kind, ''), '[^\w]', '_', 'g');
    IF safe_kind = '' OR COALESCE(p_alias_key, '') = '' OR COALESCE(p_canonical_key, '') = '' THEN
        RETURN;
    END IF;
    IF p_alias_key = p_canonical_key THEN
        RETURN;
    END IF;

    EXECUTE format(
        $cypher_exec$
        SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
            MERGE (alias:Entity {kind: %L, key: %L, tenant_id: %L})
            MERGE (canon:Entity {kind: %L, key: %L, tenant_id: %L})
            WITH alias, canon
            MATCH (m:Memory {tenant_id: %L})-[r:mentions]->(alias)
            MERGE (m)-[r2:mentions {slot: r.slot}]->(canon)
            SET r2.confidence = COALESCE(r.confidence, 0.0),
                r2.memory_version = COALESCE(r.memory_version, 0),
                r2.kind = COALESCE(r.kind, %L)
            DELETE r
            WITH alias, canon
            MERGE (alias)-[:same_as]->(canon)
        $cypher$) AS (result ag_catalog.agtype)
        $cypher_exec$,
        safe_kind, p_alias_key, p_tenant_id::text,
        safe_kind, p_canonical_key, p_tenant_id::text,
        p_tenant_id::text,
        safe_kind
    );
EXCEPTION
    WHEN others THEN
        RAISE WARNING 'merge_entity_alias_in_graph: %', SQLERRM;
END;
$fn$;

EXCEPTION WHEN undefined_file THEN
        RAISE NOTICE 'AGE extension not available — skipping entity same_as graph setup';
    WHEN SQLSTATE '58P01' THEN
        RAISE NOTICE 'AGE extension not available — skipping entity same_as graph setup';
    WHEN SQLSTATE '0A000' THEN
        RAISE NOTICE 'AGE extension not available — skipping entity same_as graph setup';
    WHEN SQLSTATE '42883' THEN
        RAISE NOTICE 'AGE entity same_as helpers not available — skipping';

END;
$outer$;
