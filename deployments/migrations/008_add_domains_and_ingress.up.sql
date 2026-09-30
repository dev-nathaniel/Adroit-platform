-- ============================================================================
-- Adroit Platform Migration (008_add_domains_and_ingress.up.sql)
-- Add custom_domains and ingress_routes tables for Caddy edge routing
-- and custom domain verification.
-- ============================================================================

-- 1. Custom Domains Table
CREATE TABLE IF NOT EXISTS custom_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending_verification',
    verification_type VARCHAR(20) NOT NULL DEFAULT 'cname',
    verification_token VARCHAR(255) NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    ssl_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    ssl_error TEXT,
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_custom_domains_domain UNIQUE (domain)
);

CREATE INDEX IF NOT EXISTS idx_custom_domains_service ON custom_domains(service_id);
CREATE INDEX IF NOT EXISTS idx_custom_domains_status ON custom_domains(status);

-- 2. Ingress Routes Table (Tracks edge reverse proxy bindings in Caddy)
CREATE TABLE IF NOT EXISTS ingress_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    domain_id UUID REFERENCES custom_domains(id) ON DELETE CASCADE,
    host_pattern VARCHAR(255) NOT NULL,
    upstream_url TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_synced_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_ingress_routes_host UNIQUE (host_pattern)
);

CREATE INDEX IF NOT EXISTS idx_ingress_routes_service ON ingress_routes(service_id);
