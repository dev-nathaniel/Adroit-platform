-- ============================================================================
-- Adroit Platform Migration (010_vault_secrets_and_config_snapshots.down.sql)
-- Revert Vault secrets metadata and deployment config snapshots
-- ============================================================================

DROP TABLE IF EXISTS deployment_env_snapshots;

DROP INDEX IF EXISTS idx_env_vars_service_target;
DROP INDEX IF EXISTS idx_env_vars_project_target;

ALTER TABLE environment_variables DROP COLUMN IF EXISTS vault_path;
ALTER TABLE environment_variables DROP COLUMN IF EXISTS target_env;
ALTER TABLE environment_variables DROP COLUMN IF EXISTS scope;
