-- ============================================================================
-- Adroit Platform Migration Rollback (004_standalone_services_and_env_vars.down.sql)
-- ============================================================================

DROP TABLE IF EXISTS environment_variables;

DROP INDEX IF EXISTS idx_services_org;
ALTER TABLE services DROP CONSTRAINT IF EXISTS uq_org_service_slug;

-- Delete any standalone services before re-enforcing project_id NOT NULL
DELETE FROM services WHERE project_id IS NULL;

ALTER TABLE services ALTER COLUMN project_id SET NOT NULL;

ALTER TABLE services DROP CONSTRAINT IF EXISTS services_project_id_fkey;
ALTER TABLE services ADD CONSTRAINT services_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE services ADD CONSTRAINT uq_project_service_slug UNIQUE (project_id, slug);

ALTER TABLE services DROP COLUMN IF EXISTS organization_id;
