-- ============================================================================
-- Adroit Platform Migration (009_add_log_service.up.sql)
-- Add app_logs table for container runtime & application log streaming
-- ============================================================================

CREATE TABLE IF NOT EXISTS app_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    deployment_id UUID REFERENCES deployments(id) ON DELETE CASCADE,
    allocation_id VARCHAR(255),
    stream VARCHAR(50) NOT NULL DEFAULT 'stdout',
    message TEXT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_app_logs_service_time ON app_logs(service_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_app_logs_deployment_time ON app_logs(deployment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_app_logs_stream ON app_logs(stream);
CREATE INDEX IF NOT EXISTS idx_app_logs_allocation ON app_logs(allocation_id);
