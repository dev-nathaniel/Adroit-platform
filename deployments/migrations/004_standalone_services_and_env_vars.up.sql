-- ============================================================================
-- Adroit Platform Migration (004_standalone_services_and_env_vars.up.sql)
-- Enable standalone services (nullable project_id, organization_id scoping),
-- move services between projects, and add environment_variables table.
-- ============================================================================

-- 1. Add organization_id to services
ALTER TABLE services ADD COLUMN IF NOT EXISTS organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- Backfill organization_id for any existing service records from projects
UPDATE services s 
SET organization_id = p.organization_id 
FROM projects p 
WHERE s.project_id = p.id AND s.organization_id IS NULL;

-- Enforce NOT NULL on organization_id
ALTER TABLE services ALTER COLUMN organization_id SET NOT NULL;

-- 2. Make project_id nullable so services can exist outside of a project
ALTER TABLE services ALTER COLUMN project_id DROP NOT NULL;

-- 3. Update foreign key to ON DELETE SET NULL so deleting a project does not delete services
ALTER TABLE services DROP CONSTRAINT IF EXISTS services_project_id_fkey;
ALTER TABLE services ADD CONSTRAINT services_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;

-- 4. Update unique slug constraint: slugs are unique per organization
ALTER TABLE services DROP CONSTRAINT IF EXISTS uq_project_service_slug;
ALTER TABLE services ADD CONSTRAINT uq_org_service_slug UNIQUE (organization_id, slug);

-- 5. Indexes for fast organization and project service queries
CREATE INDEX IF NOT EXISTS idx_services_org ON services(organization_id);

-- 6. Environment Variables (scoped to services or projects)
CREATE TABLE IF NOT EXISTS environment_variables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    service_id UUID REFERENCES services(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    is_secret BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_env_target CHECK (
        (project_id IS NOT NULL AND service_id IS NULL) OR
        (project_id IS NULL AND service_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_env_vars_org ON environment_variables(organization_id);
CREATE INDEX IF NOT EXISTS idx_env_vars_service ON environment_variables(service_id);
CREATE INDEX IF NOT EXISTS idx_env_vars_project ON environment_variables(project_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_env_service_key ON environment_variables(service_id, key) WHERE service_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_env_project_key ON environment_variables(project_id, key) WHERE project_id IS NOT NULL;
