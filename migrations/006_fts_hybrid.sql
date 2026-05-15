-- v1.9.0: full-text search column for hybrid BM25-style retrieval
ALTER TABLE memory_entries
    ADD COLUMN IF NOT EXISTS content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED;

CREATE INDEX IF NOT EXISTS idx_memories_content_fts ON memory_entries USING GIN (content_tsv);
