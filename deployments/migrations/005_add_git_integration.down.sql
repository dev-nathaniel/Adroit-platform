-- ============================================================================
-- Adroit Platform Migration Rollback (005_add_git_integration.down.sql)
-- ============================================================================

DROP TABLE IF EXISTS git_webhook_events;
DROP TABLE IF EXISTS git_providers;
DROP INDEX IF EXISTS idx_users_gitlab_id;
ALTER TABLE users DROP COLUMN IF EXISTS gitlab_access_token;
ALTER TABLE users DROP COLUMN IF EXISTS gitlab_username;
ALTER TABLE users DROP COLUMN IF EXISTS gitlab_id;
ALTER TABLE services DROP COLUMN IF EXISTS webhook_secret;
