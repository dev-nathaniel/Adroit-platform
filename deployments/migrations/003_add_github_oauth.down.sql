-- ============================================================================
-- Adroit Platform Rollback GitHub OAuth (003_add_github_oauth.down.sql)
-- ============================================================================

DROP INDEX IF EXISTS idx_users_github_id;
ALTER TABLE users DROP COLUMN IF EXISTS github_access_token;
ALTER TABLE users DROP COLUMN IF EXISTS github_username;
ALTER TABLE users DROP COLUMN IF EXISTS github_id;

-- Ensure no nulls remain before reenforcing NOT NULL constraint
UPDATE users SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
