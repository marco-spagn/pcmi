-- v2.0 Phase D: tenant entity registry, aliases, evolution snapshots, alias proposal queue.
-- Generic across all extraction profiles (SOC, CTI, CRM, …).

CREATE TABLE IF NOT EXISTS entity_registry (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    canonical_key   TEXT NOT NULL,
    display_name    TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, kind, canonical_key)
);

CREATE INDEX IF NOT EXISTS idx_entity_registry_tenant_kind
    ON entity_registry (tenant_id, kind);

CREATE TABLE IF NOT EXISTS entity_aliases (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_id       UUID NOT NULL REFERENCES entity_registry(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    alias_key       TEXT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'extraction'
        CHECK (source IN ('manual', 'extraction', 'alias_proposal')),
    confidence      FLOAT8 NOT NULL DEFAULT 1.0
        CHECK (confidence >= 0 AND confidence <= 1),
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_to        TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}'
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_aliases_active_unique
    ON entity_aliases (tenant_id, kind, alias_key)
    WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_entity_aliases_entity
    ON entity_aliases (entity_id)
    WHERE valid_to IS NULL;

CREATE TABLE IF NOT EXISTS entity_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_id       UUID NOT NULL REFERENCES entity_registry(id) ON DELETE CASCADE,
    memory_id       BIGINT NOT NULL,
    memory_version  INT NOT NULL CHECK (memory_version >= 1),
    profile_id      TEXT,
    slot            TEXT NOT NULL,
    raw_key         TEXT,
    attributes      JSONB NOT NULL DEFAULT '{}',
    confidence      FLOAT8 NOT NULL DEFAULT 0
        CHECK (confidence >= 0 AND confidence <= 1),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, entity_id, memory_id, memory_version, slot)
);

CREATE INDEX IF NOT EXISTS idx_entity_snapshots_entity_time
    ON entity_snapshots (tenant_id, entity_id, created_at DESC);

CREATE TABLE IF NOT EXISTS entity_alias_proposals (
    id                  BIGSERIAL PRIMARY KEY,
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL,
    alias_key           TEXT NOT NULL,
    source_entity_id    UUID NOT NULL REFERENCES entity_registry(id) ON DELETE CASCADE,
    target_entity_id    UUID NOT NULL REFERENCES entity_registry(id) ON DELETE CASCADE,
    source_memory_id    BIGINT,
    status              TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected')),
    confidence          FLOAT8 NOT NULL DEFAULT 0
        CHECK (confidence >= 0 AND confidence <= 1),
    reason              TEXT NOT NULL DEFAULT '',
    model               TEXT,
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_entity_alias_proposals_tenant_status
    ON entity_alias_proposals (tenant_id, status, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_alias_proposals_pending_unique
    ON entity_alias_proposals (tenant_id, kind, alias_key, target_entity_id)
    WHERE status = 'pending';

ALTER TABLE entity_registry ENABLE ROW LEVEL SECURITY;
ALTER TABLE entity_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE entity_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE entity_alias_proposals ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_entity_registry ON entity_registry
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
CREATE POLICY tenant_entity_aliases ON entity_aliases
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
CREATE POLICY tenant_entity_snapshots ON entity_snapshots
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
CREATE POLICY tenant_entity_alias_proposals ON entity_alias_proposals
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
