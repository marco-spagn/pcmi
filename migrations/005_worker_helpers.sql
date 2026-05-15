-- v1.7 Worker helpers: list pending embeddings across tenants (SECURITY DEFINER for RLS)
CREATE OR REPLACE FUNCTION list_pending_embeddings(limit_n int DEFAULT 5)
RETURNS TABLE(id bigint, content text, tenant_id uuid) AS $$
BEGIN
    RETURN QUERY
    SELECT m.id, m.content, m.tenant_id
    FROM memory_entries m
    WHERE m.embedding IS NULL
      AND m.valid_to IS NULL
    ORDER BY m.created_at ASC
    LIMIT limit_n;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;
