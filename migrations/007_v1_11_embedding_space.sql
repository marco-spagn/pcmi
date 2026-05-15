-- v1.11.0: multi embedding space label + events ingest index
ALTER TABLE memory_entries
    ADD COLUMN IF NOT EXISTS embedding_space TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_memories_tenant_embedding_space
    ON memory_entries (tenant_id, embedding_space)
    WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_events_tenant_type_time
    ON events (tenant_id, event_type, timestamp DESC);
