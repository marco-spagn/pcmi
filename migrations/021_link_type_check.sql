-- 021_link_type_check.sql — constrain memory_links.link_type to the known set.
--
-- Before this migration POST /v1/memories/links accepted any string as
-- link_type (LinksRepository.Create only defaulted empty -> 'related'), so the
-- documented five-type enum was not enforced at any layer. Validation now lives
-- in model.NormalizeLinkType on the public write path; this CHECK is the
-- database-level backstop for every other writer.
--
-- Allowed values:
--   causal, temporal, contradicts, supports, related  -- user-assignable (docs)
--   duplicate                                          -- reserved for the dedup
--                                                          engine (model.DedupLinkType,
--                                                          repository/dedup.go)
--
-- Added NOT VALID so the constraint enforces all new INSERT/UPDATEs without
-- scanning (and potentially failing on) link_type values written by older
-- builds. After confirming legacy rows conform, run:
--   ALTER TABLE memory_links VALIDATE CONSTRAINT memory_links_link_type_check;
--
-- The migrate runner wraps each file in a single transaction; the DO block keeps
-- the migration idempotent (ADD CONSTRAINT has no IF NOT EXISTS).

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'memory_links_link_type_check'
          AND conrelid = 'public.memory_links'::regclass
    ) THEN
        ALTER TABLE public.memory_links
            ADD CONSTRAINT memory_links_link_type_check
            CHECK (link_type IN (
                'causal', 'temporal', 'contradicts', 'supports', 'related', 'duplicate'
            )) NOT VALID;
    END IF;
END $$;
