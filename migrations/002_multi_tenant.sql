-- v1.4 Multi-tenant + RBAC
-- =============================================

-- 1. Tabella Tenants
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. Abilita RLS su tutte le tabelle esistenti
ALTER TABLE memory_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE distilled_knowledge ENABLE ROW LEVEL SECURITY;

-- 3. Policy per memory_entries (isolamento tenant)
CREATE POLICY tenant_isolation_memory ON memory_entries
    USING (tenant_id = current_setting('app.current_tenant')::uuid);

CREATE POLICY tenant_isolation_distilled ON distilled_knowledge
    USING (tenant_id = current_setting('app.current_tenant')::uuid);

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;

-- 4. Policy per tenants (solo admin può vedere tutti)
CREATE POLICY tenant_self ON tenants
    USING (id = current_setting('app.current_tenant')::uuid);

-- 5. Funzione helper per settare tenant nel contesto
CREATE OR REPLACE FUNCTION set_tenant_context(tenant_uuid UUID)
RETURNS void AS $$
BEGIN
    PERFORM set_config('app.current_tenant', tenant_uuid::text, false);
END;
$$ LANGUAGE plpgsql;

-- 6. Inserisci tenant di default (per compatibilità con test esistenti)
INSERT INTO tenants (id, slug, name, settings)
VALUES ('00000000-0000-0000-0000-000000000000', 'default', 'Default Tenant', '{}')
ON CONFLICT (id) DO NOTHING;

-- 7. Indici per performance
CREATE INDEX idx_memory_entries_tenant_id ON memory_entries(tenant_id);
CREATE INDEX idx_distilled_knowledge_tenant_id ON distilled_knowledge(tenant_id);
CREATE INDEX idx_tenants_slug ON tenants(slug);

