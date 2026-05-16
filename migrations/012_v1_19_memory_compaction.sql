-- v1.19.0: per-path compaction of superseded memory rows (beyond global age pruning).
-- Keeps the current row (valid_to IS NULL) and the newest N superseded versions; deletes older closed rows.

CREATE OR REPLACE FUNCTION compact_memory_path_history(
  p_tenant_id uuid,
  p_path ltree,
  p_keep_superseded int DEFAULT 20
) RETURNS int AS $$
DECLARE
  deleted_count int;
  k int;
BEGIN
  IF p_keep_superseded < 1 THEN
    RAISE EXCEPTION 'p_keep_superseded must be >= 1';
  END IF;
  IF p_keep_superseded > 500 THEN
    RAISE EXCEPTION 'p_keep_superseded must be <= 500';
  END IF;

  k := p_keep_superseded;

  WITH superseded AS (
    SELECT id,
           ROW_NUMBER() OVER (ORDER BY version DESC) AS rn
    FROM memory_entries
    WHERE tenant_id = p_tenant_id
      AND path = p_path
      AND valid_to IS NOT NULL
  ),
  doomed AS (
    SELECT id FROM superseded WHERE rn > k
  )
  DELETE FROM memory_entries me
  USING doomed d
  WHERE me.id = d.id;

  GET DIAGNOSTICS deleted_count = ROW_COUNT;
  RETURN deleted_count;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = public;
