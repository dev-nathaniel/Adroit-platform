-- ============================================================================
-- Adroit Platform Migration (011_alter_app_logs_id_varchar.up.sql)
-- Change app_logs.id column from UUID to VARCHAR(64) to support log_ prefixed IDs
-- ============================================================================

ALTER TABLE app_logs ALTER COLUMN id DROP DEFAULT;
ALTER TABLE app_logs ALTER COLUMN id TYPE VARCHAR(64) USING id::text;
