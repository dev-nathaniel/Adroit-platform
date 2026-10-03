-- ============================================================================
-- Adroit Platform Migration (011_alter_app_logs_id_varchar.down.sql)
-- Revert app_logs.id column back to UUID
-- ============================================================================

ALTER TABLE app_logs ALTER COLUMN id TYPE UUID USING id::uuid;
ALTER TABLE app_logs ALTER COLUMN id SET DEFAULT gen_random_uuid();
