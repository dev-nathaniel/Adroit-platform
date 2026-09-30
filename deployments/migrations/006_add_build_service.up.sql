-- ============================================================================
-- Adroit Platform Migration (006_add_build_service.up.sql)
-- Add error_message to builds table and create build_logs table for
-- real-time log streaming and build auditing.
-- ============================================================================

-- 1. Error message tracking for failed builds
ALTER TABLE builds ADD COLUMN IF NOT EXISTS error_message TEXT;

-- 2. Build logs table for execution trace streaming & persistence
CREATE TABLE IF NOT EXISTS build_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    build_id UUID NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    line TEXT NOT NULL,
    stream VARCHAR(10) NOT NULL DEFAULT 'stdout', -- 'stdout' or 'stderr'
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_build_logs_build ON build_logs(build_id, timestamp ASC);
