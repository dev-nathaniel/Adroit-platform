package runtimeservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// Repository defines data access operations for runtime-service
type Repository interface {
	GetService(ctx context.Context, serviceID string) (*types.Service, error)
	GetAutoscalePolicy(ctx context.Context, serviceID string) (*types.ServiceAutoScalingPolicy, error)
	UpdateAutoscalePolicy(ctx context.Context, serviceID string, req UpdateAutoscalePolicyRequest) (*types.ServiceAutoScalingPolicy, error)
	ListActiveAutoscalingServices(ctx context.Context) ([]*types.Service, error)
	RecordMetricsSample(ctx context.Context, sample *types.LiveServiceMetrics) error
	GetLatestMetrics(ctx context.Context, serviceID string) (*types.LiveServiceMetrics, error)
	GetMetricsHistory(ctx context.Context, serviceID string, limit int) ([]types.LiveServiceMetrics, error)
	RecordScalingEvent(ctx context.Context, event *types.ScalingEvent) error
	ListScalingEvents(ctx context.Context, serviceID string, limit int) ([]types.ScalingEvent, error)
	UpdateCurrentReplicas(ctx context.Context, serviceID string, replicas int, scaledAt time.Time) error
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates an initialized PostgresRepository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// GetService fetches service metadata with autoscaling columns
func (r *PostgresRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       COALESCE(source_type, 'git'), COALESCE(repo_url, ''), COALESCE(archive_url, ''), COALESCE(image_url, ''),
		       branch, root_directory, build_command, start_command, dockerfile_path, port,
		       min_replicas, max_replicas, current_replicas, auto_scaling_enabled,
		       COALESCE(target_cpu_percent, 80), COALESCE(target_memory_percent, 80), COALESCE(target_rps, 1000),
		       COALESCE(scale_up_cooldown_seconds, 30), COALESCE(scale_down_cooldown_seconds, 180),
		       last_scaled_at,
		       cpu_cores, memory_mb, disk_mb,
		       health_check_path, active_deployment_id, created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var s types.Service
	var projID, rootDir, buildCmd, startCmd, dockerfile, hcPath, activeDeploy sql.NullString
	var serviceTypeStr, tierStr, srcType, repoURL, archiveURL, imageURL string

	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&s.ID,
		&s.OrganizationID,
		&projID,
		&s.Name,
		&s.Slug,
		&serviceTypeStr,
		&tierStr,
		&srcType,
		&repoURL,
		&archiveURL,
		&imageURL,
		&s.Branch,
		&rootDir,
		&buildCmd,
		&startCmd,
		&dockerfile,
		&s.Port,
		&s.MinReplicas,
		&s.MaxReplicas,
		&s.CurrentReplicas,
		&s.AutoScalingEnabled,
		&s.TargetCPUPercent,
		&s.TargetMemoryPercent,
		&s.TargetRPS,
		&s.ScaleUpCooldownSeconds,
		&s.ScaleDownCooldownSeconds,
		&s.LastScaledAt,
		&s.Resources.CPUCores,
		&s.Resources.MemoryMB,
		&s.Resources.DiskMB,
		&hcPath,
		&activeDeploy,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("service not found: %s", serviceID)
		}
		return nil, fmt.Errorf("failed to fetch service: %w", err)
	}

	if projID.Valid {
		s.ProjectID = &projID.String
	}
	s.Type = types.ServiceType(serviceTypeStr)
	s.RuntimeTier = types.RuntimeTier(tierStr)
	s.SourceType = types.SourceType(srcType)
	s.RepoURL = repoURL
	s.ArchiveURL = archiveURL
	s.ImageURL = imageURL
	if rootDir.Valid {
		s.RootDirectory = rootDir.String
	}
	if buildCmd.Valid {
		s.BuildCommand = buildCmd.String
	}
	if startCmd.Valid {
		s.StartCommand = startCmd.String
	}
	if dockerfile.Valid {
		s.DockerfilePath = dockerfile.String
	}
	if hcPath.Valid {
		s.HealthCheck.Path = hcPath.String
	}
	if activeDeploy.Valid {
		s.ActiveDeploymentID = &activeDeploy.String
	}

	return &s, nil
}

// GetAutoscalePolicy returns the elastic scaling policy for a service
func (r *PostgresRepository) GetAutoscalePolicy(ctx context.Context, serviceID string) (*types.ServiceAutoScalingPolicy, error) {
	svc, err := r.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	return &types.ServiceAutoScalingPolicy{
		ServiceID:                svc.ID,
		AutoScalingEnabled:       svc.AutoScalingEnabled,
		MinReplicas:              svc.MinReplicas,
		MaxReplicas:              svc.MaxReplicas,
		TargetCPUPercent:         svc.TargetCPUPercent,
		TargetMemoryPercent:      svc.TargetMemoryPercent,
		TargetRPS:                svc.TargetRPS,
		ScaleUpCooldownSeconds:   svc.ScaleUpCooldownSeconds,
		ScaleDownCooldownSeconds: svc.ScaleDownCooldownSeconds,
		CurrentReplicas:          svc.CurrentReplicas,
		LastScaledAt:             svc.LastScaledAt,
	}, nil
}

// UpdateAutoscalePolicy modifies scaling parameters and elastic boundaries
func (r *PostgresRepository) UpdateAutoscalePolicy(ctx context.Context, serviceID string, req UpdateAutoscalePolicyRequest) (*types.ServiceAutoScalingPolicy, error) {
	svc, err := r.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if req.AutoScalingEnabled != nil {
		svc.AutoScalingEnabled = *req.AutoScalingEnabled
	}
	if req.MinReplicas != nil && *req.MinReplicas >= 1 {
		svc.MinReplicas = *req.MinReplicas
	}
	if req.MaxReplicas != nil && *req.MaxReplicas >= svc.MinReplicas {
		svc.MaxReplicas = *req.MaxReplicas
	}
	if req.TargetCPUPercent != nil && *req.TargetCPUPercent > 0 {
		svc.TargetCPUPercent = *req.TargetCPUPercent
	}
	if req.TargetMemoryPercent != nil && *req.TargetMemoryPercent > 0 {
		svc.TargetMemoryPercent = *req.TargetMemoryPercent
	}
	if req.TargetRPS != nil && *req.TargetRPS > 0 {
		svc.TargetRPS = *req.TargetRPS
	}
	if req.ScaleUpCooldownSeconds != nil && *req.ScaleUpCooldownSeconds >= 0 {
		svc.ScaleUpCooldownSeconds = *req.ScaleUpCooldownSeconds
	}
	if req.ScaleDownCooldownSeconds != nil && *req.ScaleDownCooldownSeconds >= 0 {
		svc.ScaleDownCooldownSeconds = *req.ScaleDownCooldownSeconds
	}

	query := `
		UPDATE services 
		SET auto_scaling_enabled = $2, min_replicas = $3, max_replicas = $4,
		    target_cpu_percent = $5, target_memory_percent = $6, target_rps = $7,
		    scale_up_cooldown_seconds = $8, scale_down_cooldown_seconds = $9,
		    updated_at = $10
		WHERE id = $1
	`
	_, err = r.db.ExecContext(ctx, query,
		serviceID,
		svc.AutoScalingEnabled,
		svc.MinReplicas,
		svc.MaxReplicas,
		svc.TargetCPUPercent,
		svc.TargetMemoryPercent,
		svc.TargetRPS,
		svc.ScaleUpCooldownSeconds,
		svc.ScaleDownCooldownSeconds,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update autoscale policy: %w", err)
	}

	return r.GetAutoscalePolicy(ctx, serviceID)
}

// ListActiveAutoscalingServices returns all deployed services with autoscaling enabled
func (r *PostgresRepository) ListActiveAutoscalingServices(ctx context.Context) ([]*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       COALESCE(source_type, 'git'), COALESCE(repo_url, ''), COALESCE(archive_url, ''), COALESCE(image_url, ''),
		       branch, root_directory, build_command, start_command, dockerfile_path, port,
		       min_replicas, max_replicas, current_replicas, auto_scaling_enabled,
		       COALESCE(target_cpu_percent, 80), COALESCE(target_memory_percent, 80), COALESCE(target_rps, 1000),
		       COALESCE(scale_up_cooldown_seconds, 30), COALESCE(scale_down_cooldown_seconds, 180),
		       last_scaled_at,
		       cpu_cores, memory_mb, disk_mb,
		       health_check_path, active_deployment_id, created_at, updated_at
		FROM services
		WHERE auto_scaling_enabled = true AND active_deployment_id IS NOT NULL
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query autoscaling services: %w", err)
	}
	defer rows.Close()

	var results []*types.Service
	for rows.Next() {
		var s types.Service
		var projID, rootDir, buildCmd, startCmd, dockerfile, hcPath, activeDeploy sql.NullString
		var serviceTypeStr, tierStr, srcType, repoURL, archiveURL, imageURL string

		if err := rows.Scan(
			&s.ID,
			&s.OrganizationID,
			&projID,
			&s.Name,
			&s.Slug,
			&serviceTypeStr,
			&tierStr,
			&srcType,
			&repoURL,
			&archiveURL,
			&imageURL,
			&s.Branch,
			&rootDir,
			&buildCmd,
			&startCmd,
			&dockerfile,
			&s.Port,
			&s.MinReplicas,
			&s.MaxReplicas,
			&s.CurrentReplicas,
			&s.AutoScalingEnabled,
			&s.TargetCPUPercent,
			&s.TargetMemoryPercent,
			&s.TargetRPS,
			&s.ScaleUpCooldownSeconds,
			&s.ScaleDownCooldownSeconds,
			&s.LastScaledAt,
			&s.Resources.CPUCores,
			&s.Resources.MemoryMB,
			&s.Resources.DiskMB,
			&hcPath,
			&activeDeploy,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan service row: %w", err)
		}

		if projID.Valid {
			s.ProjectID = &projID.String
		}
		s.Type = types.ServiceType(serviceTypeStr)
		s.RuntimeTier = types.RuntimeTier(tierStr)
		s.SourceType = types.SourceType(srcType)
		s.RepoURL = repoURL
		s.ArchiveURL = archiveURL
		s.ImageURL = imageURL
		if activeDeploy.Valid {
			s.ActiveDeploymentID = &activeDeploy.String
		}
		results = append(results, &s)
	}

	return results, nil
}

// RecordMetricsSample writes a telemetry snapshot into the time-series table
func (r *PostgresRepository) RecordMetricsSample(ctx context.Context, sample *types.LiveServiceMetrics) error {
	query := `
		INSERT INTO service_metrics_samples (
			service_id, cpu_percent, memory_usage_mb, memory_limit_mb, 
			memory_percent, requests_per_sec, active_allocations, sampled_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	sampledAt := sample.SampledAt
	if sampledAt.IsZero() {
		sampledAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, query,
		sample.ServiceID,
		sample.CPUPercent,
		sample.MemoryUsageMB,
		sample.MemoryLimitMB,
		sample.MemoryPercent,
		sample.RequestsPerSec,
		sample.ActiveAllocations,
		sampledAt,
	)
	return err
}

// GetLatestMetrics fetches the most recently sampled metrics record
func (r *PostgresRepository) GetLatestMetrics(ctx context.Context, serviceID string) (*types.LiveServiceMetrics, error) {
	query := `
		SELECT service_id, cpu_percent, memory_usage_mb, memory_limit_mb, 
		       memory_percent, requests_per_sec, active_allocations, sampled_at
		FROM service_metrics_samples
		WHERE service_id = $1
		ORDER BY sampled_at DESC
		LIMIT 1
	`
	var m types.LiveServiceMetrics
	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&m.ServiceID,
		&m.CPUPercent,
		&m.MemoryUsageMB,
		&m.MemoryLimitMB,
		&m.MemoryPercent,
		&m.RequestsPerSec,
		&m.ActiveAllocations,
		&m.SampledAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return default baseline if no samples recorded yet
			return &types.LiveServiceMetrics{
				ServiceID:         serviceID,
				CPUPercent:        0.0,
				MemoryUsageMB:     0.0,
				MemoryLimitMB:     512.0,
				MemoryPercent:     0.0,
				RequestsPerSec:    0.0,
				ActiveAllocations: 1,
				SampledAt:         time.Now().UTC(),
			}, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetMetricsHistory retrieves historical telemetry samples for chart rendering
func (r *PostgresRepository) GetMetricsHistory(ctx context.Context, serviceID string, limit int) ([]types.LiveServiceMetrics, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT service_id, cpu_percent, memory_usage_mb, memory_limit_mb, 
		       memory_percent, requests_per_sec, active_allocations, sampled_at
		FROM service_metrics_samples
		WHERE service_id = $1
		ORDER BY sampled_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []types.LiveServiceMetrics
	for rows.Next() {
		var m types.LiveServiceMetrics
		if err := rows.Scan(
			&m.ServiceID,
			&m.CPUPercent,
			&m.MemoryUsageMB,
			&m.MemoryLimitMB,
			&m.MemoryPercent,
			&m.RequestsPerSec,
			&m.ActiveAllocations,
			&m.SampledAt,
		); err != nil {
			return nil, err
		}
		samples = append(samples, m)
	}

	// Reverse to chronological order (oldest -> newest) for graphing
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}

	return samples, nil
}

// RecordScalingEvent writes an audit log entry for a replica adjustment
func (r *PostgresRepository) RecordScalingEvent(ctx context.Context, event *types.ScalingEvent) error {
	query := `
		INSERT INTO service_scaling_events (
			service_id, previous_replicas, desired_replicas, reason,
			trigger_metric, metric_value, target_value, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(ctx, query,
		event.ServiceID,
		event.PreviousReplicas,
		event.DesiredReplicas,
		event.Reason,
		event.TriggerMetric,
		event.MetricValue,
		event.TargetValue,
		event.Status,
		createdAt,
	).Scan(&event.ID)
}

// ListScalingEvents fetches paginated scaling events for a service
func (r *PostgresRepository) ListScalingEvents(ctx context.Context, serviceID string, limit int) ([]types.ScalingEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT id, service_id, previous_replicas, desired_replicas, reason,
		       trigger_metric, COALESCE(metric_value, 0), COALESCE(target_value, 0),
		       status, created_at
		FROM service_scaling_events
		WHERE service_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []types.ScalingEvent
	for rows.Next() {
		var e types.ScalingEvent
		if err := rows.Scan(
			&e.ID,
			&e.ServiceID,
			&e.PreviousReplicas,
			&e.DesiredReplicas,
			&e.Reason,
			&e.TriggerMetric,
			&e.MetricValue,
			&e.TargetValue,
			&e.Status,
			&e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	return events, nil
}

// UpdateCurrentReplicas updates running instance count and timestamp in database
func (r *PostgresRepository) UpdateCurrentReplicas(ctx context.Context, serviceID string, replicas int, scaledAt time.Time) error {
	query := `
		UPDATE services 
		SET current_replicas = $2, last_scaled_at = $3, updated_at = $3
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, serviceID, replicas, scaledAt.UTC())
	return err
}
