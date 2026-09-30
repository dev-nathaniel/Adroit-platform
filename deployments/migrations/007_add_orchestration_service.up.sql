-- ============================================================================
-- Adroit Platform Migration (007_add_orchestration_service.up.sql)
-- Add rollout strategy, canary state, job spec to deployments;
-- create allocations and deployment_events tables;
-- add active_deployment_id to services.
-- ============================================================================

-- 1. Extend deployments table for Nomad orchestration & rollout tracking
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS strategy VARCHAR(50) NOT NULL DEFAULT 'rolling';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS canary_promoted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS job_spec TEXT;

-- 2. Allocations table (Nomad client allocation tracking)
CREATE TABLE IF NOT EXISTS allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    nomad_alloc_id VARCHAR(255) NOT NULL,
    node_name VARCHAR(255),
    client_status VARCHAR(50) NOT NULL DEFAULT 'running',
    desired_status VARCHAR(50) NOT NULL DEFAULT 'run',
    is_healthy BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_allocations_deployment ON allocations(deployment_id);
CREATE INDEX IF NOT EXISTS idx_allocations_nomad_alloc ON allocations(nomad_alloc_id);

-- 3. Deployment Events table (Rollout milestones, canary transitions, health checks)
CREATE TABLE IF NOT EXISTS deployment_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    message TEXT NOT NULL,
    details JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_deployment_events_deployment ON deployment_events(deployment_id, created_at ASC);

-- 4. Active deployment pointer on services
ALTER TABLE services ADD COLUMN IF NOT EXISTS active_deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_services_active_deployment ON services(active_deployment_id);
