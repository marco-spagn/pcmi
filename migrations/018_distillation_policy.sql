-- v1.45.0: automatic distillation policy engine (PCMI-012)

CREATE TABLE IF NOT EXISTS distillation_policies (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    path_prefix LTREE NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    count_threshold INT NOT NULL DEFAULT 10 CHECK (count_threshold >= 1),
    min_interval_secs INT NOT NULL DEFAULT 300 CHECK (min_interval_secs >= 0),
    max_age_secs INT CHECK (max_age_secs IS NULL OR max_age_secs > 0),
    last_triggered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_distillation_policies_tenant_enabled
    ON distillation_policies (tenant_id)
    WHERE enabled = true;

CREATE TABLE IF NOT EXISTS distillation_runs (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES distillation_policies(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    path_prefix TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped')),
    error_message TEXT,
    source_count INT NOT NULL DEFAULT 0,
    distilled_id BIGINT REFERENCES distilled_knowledge(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_distillation_runs_policy
    ON distillation_runs (policy_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_distillation_runs_tenant
    ON distillation_runs (tenant_id, created_at DESC);

ALTER TABLE distillation_policies ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_distillation_policies ON distillation_policies
    USING (tenant_id = current_setting('app.current_tenant')::uuid);

ALTER TABLE distillation_runs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_distillation_runs ON distillation_runs
    USING (tenant_id = current_setting('app.current_tenant')::uuid);
