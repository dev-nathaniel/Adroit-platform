-- ============================================================================
-- Adroit Platform GitHub OAuth Schema Migration (003_add_github_oauth.up.sql)
-- ============================================================================

-- Allow password_hash to be nullable for OAuth-authenticated users
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- Add GitHub identity and token tracking columns
ALTER TABLE users ADD COLUMN IF NOT EXISTS github_id VARCHAR(255) UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS github_username VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS github_access_token TEXT;

-- Create lookup index for fast GitHub OAuth identity resolution
CREATE INDEX IF NOT EXISTS idx_users_github_id ON users(github_id);
