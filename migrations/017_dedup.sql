-- v1.44.0: content-hash deduplication at ingest (PCMI-011)

ALTER TABLE memory_entries
    ADD COLUMN IF NOT EXISTS content_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_memories_content_hash_current
    ON memory_entries (tenant_id, content_hash)
    WHERE valid_to IS NULL AND content_hash IS NOT NULL;
