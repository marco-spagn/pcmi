CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 1. Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    settings JSONB DEFAULT '{}'
);

-- 2. Agents
CREATE TABLE agents (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version TEXT,
    capabilities JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 3. Memory Entries (embedding nullable)
CREATE TABLE memory_entries (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    path LTREE NOT NULL,
    content TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',
    embedding VECTOR(1536),
    embedding_model TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_to TIMESTAMPTZ,
    source_agent_id UUID,
    source_event_id UUID,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 4. Distilled Knowledge
CREATE TABLE distilled_knowledge (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    path LTREE NOT NULL,
    summary TEXT NOT NULL,
    insights JSONB NOT NULL,
    confidence_score FLOAT,
    distilled_at TIMESTAMPTZ DEFAULT NOW(),
    source_entry_ids BIGINT[] NOT NULL,
    distillation_job_id UUID
);

-- 5. Events
CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    agent_id UUID REFERENCES agents(id)
);

-- 6. Roles
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID,
    name TEXT NOT NULL,
    permissions JSONB
);

-- Indexes
CREATE INDEX idx_memories_tenant_path ON memory_entries (tenant_id, path);
CREATE INDEX idx_memories_path_gist ON memory_entries USING GIST (path);
CREATE INDEX idx_memories_embedding_hnsw ON memory_entries USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_memories_valid_time ON memory_entries (valid_from, valid_to);
CREATE INDEX idx_events_type_time ON events (event_type, timestamp);
CREATE INDEX idx_distilled_path ON distilled_knowledge USING GIST (path);

CREATE VIEW current_memories AS
SELECT * FROM memory_entries WHERE valid_to IS NULL;

-- ==================== DEFAULT TENANT (FIX FK) ====================
INSERT INTO tenants (id, slug, name)
VALUES ('00000000-0000-0000-0000-000000000000', 'default', 'Default Tenant')
ON CONFLICT (id) DO NOTHING;