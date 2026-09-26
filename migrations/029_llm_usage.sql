-- v1.52 LLM / embedding usage metering (FinOps)
-- ============================================
--
-- Daily per-tenant token counters, written by internal/usage.Aggregator in the
-- API and worker processes (additive UPSERTs, so several processes can flush
-- the same bucket). Cost is NOT stored: GET /v1/stats/usage prices tokens at
-- read time with the operator's LLM_PRICING table.
--
-- Idempotent: safe to re-apply.

CREATE TABLE IF NOT EXISTS llm_usage_daily (
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    day           DATE NOT NULL,
    operation     TEXT NOT NULL,
    provider      TEXT NOT NULL,
    model         TEXT NOT NULL,
    requests      BIGINT NOT NULL DEFAULT 0 CHECK (requests >= 0),
    input_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, day, operation, provider, model)
);

CREATE INDEX IF NOT EXISTS idx_llm_usage_daily_day ON llm_usage_daily (day);

ALTER TABLE llm_usage_daily ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS llm_usage_daily_tenant_isolation ON llm_usage_daily;
CREATE POLICY llm_usage_daily_tenant_isolation ON llm_usage_daily
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
