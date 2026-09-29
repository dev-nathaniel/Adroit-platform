-- ============================================================================
-- Adroit Platform Rollback Schema Migration (001_init_schema.down.sql)
-- ============================================================================

DROP TABLE IF EXISTS deployments CASCADE;
DROP TABLE IF EXISTS builds CASCADE;
DROP TABLE IF EXISTS services CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS api_tokens CASCADE;
DROP TABLE IF EXISTS organization_members CASCADE;
DROP TABLE IF EXISTS organizations CASCADE;
DROP TABLE IF EXISTS users CASCADE;
