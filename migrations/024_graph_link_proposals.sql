-- v2.0 Phase C: LLM memory link proposals (review queue before materializing to memory_links).

CREATE TABLE IF NOT EXISTS graph_link_proposals (
    id                 BIGSERIAL PRIMARY KEY,
    tenant_id          UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_memory_id   BIGINT NOT NULL,
    from_memory_id     BIGINT NOT NULL,
    to_memory_id       BIGINT NOT NULL,
    from_path          LTREE NOT NULL,
    to_path            LTREE NOT NULL,
    link_type          TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected')),
    confidence         FLOAT8 NOT NULL DEFAULT 0
        CHECK (confidence >= 0 AND confidence <= 1),
    reason             TEXT NOT NULL DEFAULT '',
    profile_id         TEXT,
    model              TEXT,
    metadata           JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_graph_link_proposals_tenant_status
    ON graph_link_proposals (tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_graph_link_proposals_source
    ON graph_link_proposals (tenant_id, source_memory_id)
    WHERE status = 'pending';

-- At most one pending proposal per directed edge + link type.
CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_link_proposals_pending_unique
    ON graph_link_proposals (tenant_id, from_path, to_path, link_type)
    WHERE status = 'pending';

ALTER TABLE graph_link_proposals ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_graph_link_proposals ON graph_link_proposals
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
