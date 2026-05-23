-- v1.43.0: agent sessions and working memory (metadata.session_id)

CREATE TABLE IF NOT EXISTS agent_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id    UUID REFERENCES agents(id),
    metadata    JSONB NOT NULL DEFAULT '{}',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_tenant_started
    ON agent_sessions (tenant_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent
    ON agent_sessions (agent_id)
    WHERE agent_id IS NOT NULL;

-- Fast lookup of working-memory rows scoped to a session (current versions only).
CREATE INDEX IF NOT EXISTS idx_memory_entries_session_id
    ON memory_entries ((metadata->>'session_id'))
    WHERE valid_to IS NULL AND metadata->>'session_id' IS NOT NULL;

ALTER TABLE agent_sessions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS agent_sessions_tenant_isolation ON agent_sessions;
CREATE POLICY agent_sessions_tenant_isolation ON agent_sessions
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
