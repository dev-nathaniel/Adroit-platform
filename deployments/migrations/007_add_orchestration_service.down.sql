-- ============================================================================
-- Adroit Platform Migration (007_add_orchestration_service.down.sql)
-- Rollback orchestration schema extensions
-- ============================================================================

ALTER TABLE services DROP COLUMN IF EXISTS active_deployment_id;
DROP TABLE IF EXISTS deployment_events CASCADE;
DROP TABLE IF EXISTS allocations CASCADE;
ALTER TABLE deployments DROP COLUMN IF EXISTS job_spec;
ALTER TABLE deployments DROP COLUMN IF EXISTS canary_promoted;
ALTER TABLE deployments DROP COLUMN IF EXISTS strategy;
