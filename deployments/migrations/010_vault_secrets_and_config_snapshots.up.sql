-- ============================================================================
-- Adroit Platform Migration (010_vault_secrets_and_config_snapshots.up.sql)
-- Add scope, target_env, vault_path to environment_variables
-- Add deployment_env_snapshots for immutable configuration releases
-- ============================================================================

-- 1. Extend environment_variables with scope, target_env, and vault_path
ALTER TABLE environment_variables ADD COLUMN IF NOT EXISTS scope VARCHAR(20) NOT NULL DEFAULT 'all';
ALTER TABLE environment_variables ADD COLUMN IF NOT EXISTS target_env VARCHAR(30) NOT NULL DEFAULT 'all';
ALTER TABLE environment_variables ADD COLUMN IF NOT EXISTS vault_path VARCHAR(500);

-- Indexes for target environment queries
CREATE INDEX IF NOT EXISTS idx_env_vars_service_target ON environment_variables(service_id, target_env);
CREATE INDEX IF NOT EXISTS idx_env_vars_project_target ON environment_variables(project_id, target_env);

-- 2. Deployment Environment Snapshots (Immutable Configuration Releases)
CREATE TABLE IF NOT EXISTS deployment_env_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    config_release_num INT NOT NULL DEFAULT 1,
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_keys JSONB NOT NULL DEFAULT '[]'::jsonb,
    vault_snapshot_path VARCHAR(500),
    target_env VARCHAR(30) NOT NULL DEFAULT 'production',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_deployment_env UNIQUE (deployment_id)
);

CREATE INDEX IF NOT EXISTS idx_deploy_env_service ON deployment_env_snapshots(service_id);
CREATE INDEX IF NOT EXISTS idx_deploy_env_deployment ON deployment_env_snapshots(deployment_id);
