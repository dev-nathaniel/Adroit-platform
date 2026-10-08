-- ============================================================================
-- Adroit Platform Migration (013_add_autoscaling_and_runtime_metrics.up.sql)
-- Add auto-scaling policy attributes to services table.
-- Create service_scaling_events table for tracking scaling decisions & audit logs.
-- Create service_metrics_samples table for time-series container telemetry.
-- ============================================================================

-- 1. Add auto-scaling policy columns to services
ALTER TABLE services ADD COLUMN IF NOT EXISTS auto_scaling_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE services ADD COLUMN IF NOT EXISTS target_cpu_percent INT DEFAULT 80;
ALTER TABLE services ADD COLUMN IF NOT EXISTS target_memory_percent INT DEFAULT 80;
ALTER TABLE services ADD COLUMN IF NOT EXISTS target_rps INT DEFAULT 1000;
ALTER TABLE services ADD COLUMN IF NOT EXISTS current_replicas INT NOT NULL DEFAULT 1;
ALTER TABLE services ADD COLUMN IF NOT EXISTS scale_up_cooldown_seconds INT NOT NULL DEFAULT 30;
ALTER TABLE services ADD COLUMN IF NOT EXISTS scale_down_cooldown_seconds INT NOT NULL DEFAULT 180;
ALTER TABLE services ADD COLUMN IF NOT EXISTS last_scaled_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- 2. Populate current_replicas with existing min_replicas value
UPDATE services SET current_replicas = min_replicas WHERE current_replicas IS NULL OR current_replicas = 1;

-- 3. Scaling events audit table
CREATE TABLE IF NOT EXISTS service_scaling_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    previous_replicas INT NOT NULL,
    desired_replicas INT NOT NULL,
    reason TEXT NOT NULL,
    trigger_metric VARCHAR(50) NOT NULL, -- 'cpu', 'memory', 'rps', 'manual'
    metric_value NUMERIC(10, 2),
    target_value NUMERIC(10, 2),
    status VARCHAR(50) NOT NULL DEFAULT 'applied', -- 'applied', 'failed', 'cooldown_skipped'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scaling_events_service ON service_scaling_events(service_id, created_at DESC);

-- 4. Service metrics samples table (time-series telemetry)
CREATE TABLE IF NOT EXISTS service_metrics_samples (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    cpu_percent NUMERIC(5, 2) NOT NULL DEFAULT 0,
    memory_usage_mb NUMERIC(10, 2) NOT NULL DEFAULT 0,
    memory_limit_mb NUMERIC(10, 2) NOT NULL DEFAULT 0,
    memory_percent NUMERIC(5, 2) NOT NULL DEFAULT 0,
    requests_per_sec NUMERIC(10, 2) NOT NULL DEFAULT 0,
    active_allocations INT NOT NULL DEFAULT 1,
    sampled_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_metrics_samples_service ON service_metrics_samples(service_id, sampled_at DESC);
