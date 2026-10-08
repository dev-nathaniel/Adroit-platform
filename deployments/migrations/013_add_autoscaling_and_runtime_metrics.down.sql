-- ============================================================================
-- Adroit Platform Migration (013_add_autoscaling_and_runtime_metrics.down.sql)
-- Revert auto-scaling and runtime metrics schema additions.
-- ============================================================================

DROP TABLE IF EXISTS service_metrics_samples;
DROP TABLE IF EXISTS service_scaling_events;

ALTER TABLE services DROP COLUMN IF EXISTS last_scaled_at;
ALTER TABLE services DROP COLUMN IF EXISTS scale_down_cooldown_seconds;
ALTER TABLE services DROP COLUMN IF EXISTS scale_up_cooldown_seconds;
ALTER TABLE services DROP COLUMN IF EXISTS current_replicas;
ALTER TABLE services DROP COLUMN IF EXISTS target_rps;
ALTER TABLE services DROP COLUMN IF EXISTS target_memory_percent;
ALTER TABLE services DROP COLUMN IF EXISTS target_cpu_percent;
ALTER TABLE services DROP COLUMN IF EXISTS auto_scaling_enabled;
