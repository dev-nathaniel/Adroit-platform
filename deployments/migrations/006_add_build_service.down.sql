-- ============================================================================
-- Adroit Platform Migration (006_add_build_service.down.sql)
-- Revert build_logs table and error_message column on builds table.
-- ============================================================================

DROP TABLE IF EXISTS build_logs;
ALTER TABLE builds DROP COLUMN IF EXISTS error_message;
