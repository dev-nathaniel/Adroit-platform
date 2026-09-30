-- ============================================================================
-- Adroit Platform Migration (005_add_git_integration.up.sql)
-- Add GitLab identity columns, git_providers connection vault,
-- per-service webhook secrets, and webhook audit log.
-- ============================================================================

-- 1. Per-service webhook secret for GitHub/GitLab webhook signature validation
ALTER TABLE services ADD COLUMN IF NOT EXISTS webhook_secret VARCHAR(255);

-- 2. Add GitLab identity and token tracking columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS gitlab_id VARCHAR(255) UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS gitlab_username VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS gitlab_access_token TEXT;
CREATE INDEX IF NOT EXISTS idx_users_gitlab_id ON users(gitlab_id);

-- 3. Dedicated git_providers table for multi-account GitHub & GitLab integrations
CREATE TABLE IF NOT EXISTS git_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL, -- 'github' or 'gitlab'
    username VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    avatar_url TEXT,
    access_token TEXT NOT NULL,
    base_url TEXT, -- custom instance url (e.g. self-hosted GitLab)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_user_provider UNIQUE (user_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_git_providers_user ON git_providers(user_id);

-- 4. Audit trail for incoming Git webhooks (GitHub & GitLab push / PR / test events)
CREATE TABLE IF NOT EXISTS git_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL, -- 'github' or 'gitlab'
    event_type VARCHAR(100) NOT NULL, -- 'push', 'merge_request', 'ping', etc.
    delivery_id VARCHAR(255),
    repo_url TEXT NOT NULL,
    branch VARCHAR(255),
    commit_hash VARCHAR(64),
    commit_message TEXT,
    author VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'processed', -- 'processed', 'ignored', 'failed'
    error_message TEXT,
    matched_services JSONB DEFAULT '[]'::jsonb,
    triggered_builds JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_git_webhook_events_repo ON git_webhook_events(repo_url);
CREATE INDEX IF NOT EXISTS idx_git_webhook_events_created ON git_webhook_events(created_at DESC);
