package orchestrationservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrDeploymentNotFound = errors.New("deployment not found")
	ErrServiceNotFound    = errors.New("service not found")
	ErrBuildNotFound      = errors.New("build not found")
	ErrNoSucceededBuild   = errors.New("no succeeded build available for service deployment")
)

// Repository encapsulates all database interactions for the Orchestration Service
type Repository interface {
	CreateDeployment(ctx context.Context, d *types.Deployment) error
	GetDeployment(ctx context.Context, id string) (*types.Deployment, error)
	GetLatestDeploymentForService(ctx context.Context, serviceID string) (*types.Deployment, error)
	GetLatestSucceededBuildForService(ctx context.Context, serviceID string) (*types.Build, error)
	ListDeploymentsByService(ctx context.Context, serviceID string, limit, offset int) ([]types.Deployment, int, error)
	UpdateDeploymentStatus(ctx context.Context, id string, status types.DeploymentStatus, nomadJobID, activeURL, errorMsg string) error
	UpdateDeploymentJobSpec(ctx context.Context, id string, spec string) error
	UpdateDeploymentCanaryPromoted(ctx context.Context, id string, promoted bool) error
	UpsertAllocation(ctx context.Context, a *types.Allocation) error
	ListAllocations(ctx context.Context, deploymentID string) ([]types.Allocation, error)
	CreateDeploymentEvent(ctx context.Context, event *types.DeploymentEvent) error
	ListDeploymentEvents(ctx context.Context, deploymentID string) ([]types.DeploymentEvent, error)
	GetService(ctx context.Context, serviceID string) (*types.Service, error)
	GetBuild(ctx context.Context, buildID string) (*types.Build, error)
	GetEnvironmentVariables(ctx context.Context, serviceID string, projectID *string) (map[string]string, error)
	GetEnvironmentVariablesDetailed(ctx context.Context, serviceID string, projectID *string) (nonSecret map[string]string, secretKeys []string, err error)
	CreateDeploymentEnvSnapshot(ctx context.Context, snapshot *types.DeploymentEnvSnapshot) error
	GetDeploymentEnvSnapshot(ctx context.Context, deploymentID string) (*types.DeploymentEnvSnapshot, error)
	UpdateServiceActiveDeployment(ctx context.Context, serviceID, deploymentID string) error
	UpdateServiceReplicas(ctx context.Context, serviceID string, replicas int) error
	MarkPreviousDeploymentsReplaced(ctx context.Context, serviceID, currentDeploymentID string) error
}

// PostgresRepository implements Repository using standard PostgreSQL connection pool
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates an initialized PostgresRepository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// CreateDeployment inserts a new deployment record
func (r *PostgresRepository) CreateDeployment(ctx context.Context, d *types.Deployment) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.Replicas <= 0 {
		d.Replicas = 1
	}
	if d.Strategy == "" {
		d.Strategy = types.RolloutStrategyRolling
	}

	query := `
		INSERT INTO deployments (
			id, service_id, build_id, status, nomad_job_id, active_url, 
			replicas, strategy, canary_promoted, job_spec, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		d.ID,
		d.ServiceID,
		d.BuildID,
		string(d.Status),
		d.NomadJobID,
		d.ActiveURL,
		d.Replicas,
		string(d.Strategy),
		d.CanaryPromoted,
		d.JobSpec,
		d.ErrorMessage,
		d.CreatedAt,
		d.UpdatedAt,
	)
	return err
}

// GetDeployment fetches a deployment by primary key
func (r *PostgresRepository) GetDeployment(ctx context.Context, id string) (*types.Deployment, error) {
	query := `
		SELECT id, service_id, build_id, status, nomad_job_id, active_url, 
		       replicas, strategy, canary_promoted, job_spec, error_message, created_at, updated_at
		FROM deployments
		WHERE id = $1
	`
	var d types.Deployment
	var statusStr, strategyStr string
	var nomadJobID, activeURL, jobSpec, errorMsg sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&d.ID,
		&d.ServiceID,
		&d.BuildID,
		&statusStr,
		&nomadJobID,
		&activeURL,
		&d.Replicas,
		&strategyStr,
		&d.CanaryPromoted,
		&jobSpec,
		&errorMsg,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeploymentNotFound
		}
		return nil, err
	}

	d.Status = types.DeploymentStatus(statusStr)
	d.Strategy = types.RolloutStrategy(strategyStr)
	if nomadJobID.Valid {
		d.NomadJobID = nomadJobID.String
	}
	if activeURL.Valid {
		d.ActiveURL = activeURL.String
	}
	if jobSpec.Valid {
		d.JobSpec = jobSpec.String
	}
	if errorMsg.Valid {
		d.ErrorMessage = errorMsg.String
	}

	return &d, nil
}

// GetLatestDeploymentForService fetches the most recently created deployment for a service
func (r *PostgresRepository) GetLatestDeploymentForService(ctx context.Context, serviceID string) (*types.Deployment, error) {
	query := `
		SELECT id, service_id, build_id, status, nomad_job_id, active_url, 
		       replicas, strategy, canary_promoted, job_spec, error_message, created_at, updated_at
		FROM deployments
		WHERE service_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var d types.Deployment
	var statusStr, strategyStr string
	var nomadJobID, activeURL, jobSpec, errorMsg sql.NullString

	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&d.ID,
		&d.ServiceID,
		&d.BuildID,
		&statusStr,
		&nomadJobID,
		&activeURL,
		&d.Replicas,
		&strategyStr,
		&d.CanaryPromoted,
		&jobSpec,
		&errorMsg,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeploymentNotFound
		}
		return nil, err
	}

	d.Status = types.DeploymentStatus(statusStr)
	d.Strategy = types.RolloutStrategy(strategyStr)
	if nomadJobID.Valid {
		d.NomadJobID = nomadJobID.String
	}
	if activeURL.Valid {
		d.ActiveURL = activeURL.String
	}
	if jobSpec.Valid {
		d.JobSpec = jobSpec.String
	}
	if errorMsg.Valid {
		d.ErrorMessage = errorMsg.String
	}

	return &d, nil
}

// GetLatestSucceededBuildForService fetches the newest successful build artifact for a service
func (r *PostgresRepository) GetLatestSucceededBuildForService(ctx context.Context, serviceID string) (*types.Build, error) {
	query := `
		SELECT id, service_id, commit_hash, branch, message, author, status, 
		       image_tag, image_digest, duration_ms, triggered_by, error_message, created_at, finished_at
		FROM builds
		WHERE service_id = $1 AND status = 'succeeded'
		ORDER BY created_at DESC
		LIMIT 1
	`
	var b types.Build
	var statusStr string
	var imageTag, imageDigest, author, msg, triggeredBy, errorMsg sql.NullString
	var durationMs sql.NullInt64
	var finishedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&b.ID,
		&b.ServiceID,
		&b.CommitHash,
		&b.Branch,
		&msg,
		&author,
		&statusStr,
		&imageTag,
		&imageDigest,
		&durationMs,
		&triggeredBy,
		&errorMsg,
		&b.CreatedAt,
		&finishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoSucceededBuild
		}
		return nil, err
	}

	b.Status = types.BuildStatus(statusStr)
	if msg.Valid {
		b.Message = msg.String
	}
	if author.Valid {
		b.Author = author.String
	}
	if imageTag.Valid {
		b.ImageTag = imageTag.String
	}
	if imageDigest.Valid {
		b.ImageDigest = imageDigest.String
	}
	if durationMs.Valid {
		b.DurationMs = durationMs.Int64
	}
	if triggeredBy.Valid {
		b.TriggeredBy = triggeredBy.String
	}
	if errorMsg.Valid {
		b.ErrorMessage = errorMsg.String
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		b.FinishedAt = &t
	}

	return &b, nil
}

// ListDeploymentsByService queries deployments for a service with pagination
func (r *PostgresRepository) ListDeploymentsByService(ctx context.Context, serviceID string, limit, offset int) ([]types.Deployment, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := `SELECT COUNT(*) FROM deployments WHERE service_id = $1`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, serviceID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, service_id, build_id, status, nomad_job_id, active_url, 
		       replicas, strategy, canary_promoted, job_spec, error_message, created_at, updated_at
		FROM deployments
		WHERE service_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []types.Deployment
	for rows.Next() {
		var d types.Deployment
		var statusStr, strategyStr string
		var nomadJobID, activeURL, jobSpec, errorMsg sql.NullString

		if err := rows.Scan(
			&d.ID,
			&d.ServiceID,
			&d.BuildID,
			&statusStr,
			&nomadJobID,
			&activeURL,
			&d.Replicas,
			&strategyStr,
			&d.CanaryPromoted,
			&jobSpec,
			&errorMsg,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}

		d.Status = types.DeploymentStatus(statusStr)
		d.Strategy = types.RolloutStrategy(strategyStr)
		if nomadJobID.Valid {
			d.NomadJobID = nomadJobID.String
		}
		if activeURL.Valid {
			d.ActiveURL = activeURL.String
		}
		if jobSpec.Valid {
			d.JobSpec = jobSpec.String
		}
		if errorMsg.Valid {
			d.ErrorMessage = errorMsg.String
		}

		list = append(list, d)
	}

	return list, total, rows.Err()
}

// UpdateDeploymentStatus updates execution status, active url, nomad job ID, and error message
func (r *PostgresRepository) UpdateDeploymentStatus(ctx context.Context, id string, status types.DeploymentStatus, nomadJobID, activeURL, errorMsg string) error {
	query := `
		UPDATE deployments
		SET status = $2,
		    nomad_job_id = COALESCE(NULLIF($3, ''), nomad_job_id),
		    active_url = COALESCE(NULLIF($4, ''), active_url),
		    error_message = COALESCE(NULLIF($5, ''), error_message),
		    updated_at = $6
		WHERE id = $1
	`
	res, err := r.db.ExecContext(ctx, query, id, string(status), nomadJobID, activeURL, errorMsg, time.Now().UTC())
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDeploymentNotFound
	}
	return nil
}

// UpdateDeploymentJobSpec stores the rendered Nomad HCL job specification
func (r *PostgresRepository) UpdateDeploymentJobSpec(ctx context.Context, id string, spec string) error {
	query := `UPDATE deployments SET job_spec = $2, updated_at = $3 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, spec, time.Now().UTC())
	return err
}

// UpdateDeploymentCanaryPromoted flags a canary deployment as promoted
func (r *PostgresRepository) UpdateDeploymentCanaryPromoted(ctx context.Context, id string, promoted bool) error {
	query := `UPDATE deployments SET canary_promoted = $2, updated_at = $3 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, promoted, time.Now().UTC())
	return err
}

// UpsertAllocation records or updates a Nomad allocation record
func (r *PostgresRepository) UpsertAllocation(ctx context.Context, a *types.Allocation) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	// Check if allocation with nomad_alloc_id exists
	var existingID string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM allocations WHERE nomad_alloc_id = $1`, a.NomadAllocID).Scan(&existingID)
	if err == nil {
		// Update
		query := `
			UPDATE allocations
			SET node_name = $2, client_status = $3, desired_status = $4, is_healthy = $5, updated_at = $6
			WHERE id = $1
		`
		_, err := r.db.ExecContext(ctx, query, existingID, a.NodeName, a.ClientStatus, a.DesiredStatus, a.IsHealthy, a.UpdatedAt)
		return err
	}

	// Insert
	query := `
		INSERT INTO allocations (id, deployment_id, nomad_alloc_id, node_name, client_status, desired_status, is_healthy, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = r.db.ExecContext(ctx, query,
		a.ID,
		a.DeploymentID,
		a.NomadAllocID,
		a.NodeName,
		a.ClientStatus,
		a.DesiredStatus,
		a.IsHealthy,
		a.CreatedAt,
		a.UpdatedAt,
	)
	return err
}

// ListAllocations retrieves allocations associated with a deployment
func (r *PostgresRepository) ListAllocations(ctx context.Context, deploymentID string) ([]types.Allocation, error) {
	query := `
		SELECT id, deployment_id, nomad_alloc_id, node_name, client_status, desired_status, is_healthy, created_at, updated_at
		FROM allocations
		WHERE deployment_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allocs []types.Allocation
	for rows.Next() {
		var a types.Allocation
		var nodeName sql.NullString
		if err := rows.Scan(
			&a.ID,
			&a.DeploymentID,
			&a.NomadAllocID,
			&nodeName,
			&a.ClientStatus,
			&a.DesiredStatus,
			&a.IsHealthy,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if nodeName.Valid {
			a.NodeName = nodeName.String
		}
		allocs = append(allocs, a)
	}

	return allocs, rows.Err()
}

// CreateDeploymentEvent writes a milestone event to the rollout audit timeline
func (r *PostgresRepository) CreateDeploymentEvent(ctx context.Context, event *types.DeploymentEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	var detailsJSON []byte
	if event.Details != nil {
		detailsJSON, _ = json.Marshal(event.Details)
	}

	query := `
		INSERT INTO deployment_events (id, deployment_id, event_type, message, details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query,
		event.ID,
		event.DeploymentID,
		event.EventType,
		event.Message,
		detailsJSON,
		event.CreatedAt,
	)
	return err
}

// ListDeploymentEvents returns historical rollout events ordered chronologically
func (r *PostgresRepository) ListDeploymentEvents(ctx context.Context, deploymentID string) ([]types.DeploymentEvent, error) {
	query := `
		SELECT id, deployment_id, event_type, message, details, created_at
		FROM deployment_events
		WHERE deployment_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []types.DeploymentEvent
	for rows.Next() {
		var e types.DeploymentEvent
		var detailsRaw []byte
		if err := rows.Scan(&e.ID, &e.DeploymentID, &e.EventType, &e.Message, &detailsRaw, &e.CreatedAt); err != nil {
			return nil, err
		}
		if len(detailsRaw) > 0 {
			_ = json.Unmarshal(detailsRaw, &e.Details)
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

// GetService fetches a service definition by ID
func (r *PostgresRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier, repo_url, branch,
		       root_directory, build_command, start_command, dockerfile_path, port,
		       min_replicas, max_replicas, cpu_cores, memory_mb, disk_mb,
		       health_check_path, active_deployment_id, created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var s types.Service
	var projID, rootDir, buildCmd, startCmd, dockerfile, hcPath, activeDeploy sql.NullString
	var serviceTypeStr, tierStr string

	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&s.ID,
		&s.OrganizationID,
		&projID,
		&s.Name,
		&s.Slug,
		&serviceTypeStr,
		&tierStr,
		&s.RepoURL,
		&s.Branch,
		&rootDir,
		&buildCmd,
		&startCmd,
		&dockerfile,
		&s.Port,
		&s.MinReplicas,
		&s.MaxReplicas,
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
			return nil, ErrServiceNotFound
		}
		return nil, err
	}

	s.Type = types.ServiceType(serviceTypeStr)
	s.RuntimeTier = types.RuntimeTier(tierStr)
	if projID.Valid {
		val := projID.String
		s.ProjectID = &val
	}
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
		val := activeDeploy.String
		s.ActiveDeploymentID = &val
	}

	return &s, nil
}

// GetBuild fetches build metadata by build ID
func (r *PostgresRepository) GetBuild(ctx context.Context, buildID string) (*types.Build, error) {
	query := `
		SELECT id, service_id, commit_hash, branch, message, author, status, 
		       image_tag, image_digest, duration_ms, triggered_by, error_message, created_at, finished_at
		FROM builds
		WHERE id = $1
	`
	var b types.Build
	var statusStr string
	var imageTag, imageDigest, author, msg, triggeredBy, errorMsg sql.NullString
	var durationMs sql.NullInt64
	var finishedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, buildID).Scan(
		&b.ID,
		&b.ServiceID,
		&b.CommitHash,
		&b.Branch,
		&msg,
		&author,
		&statusStr,
		&imageTag,
		&imageDigest,
		&durationMs,
		&triggeredBy,
		&errorMsg,
		&b.CreatedAt,
		&finishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBuildNotFound
		}
		return nil, err
	}

	b.Status = types.BuildStatus(statusStr)
	if msg.Valid {
		b.Message = msg.String
	}
	if author.Valid {
		b.Author = author.String
	}
	if imageTag.Valid {
		b.ImageTag = imageTag.String
	}
	if imageDigest.Valid {
		b.ImageDigest = imageDigest.String
	}
	if durationMs.Valid {
		b.DurationMs = durationMs.Int64
	}
	if triggeredBy.Valid {
		b.TriggeredBy = triggeredBy.String
	}
	if errorMsg.Valid {
		b.ErrorMessage = errorMsg.String
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		b.FinishedAt = &t
	}

	return &b, nil
}

// GetEnvironmentVariables loads environment variables for service & optional parent project
func (r *PostgresRepository) GetEnvironmentVariables(ctx context.Context, serviceID string, projectID *string) (map[string]string, error) {
	envVars := make(map[string]string)

	// 1. Load project-level env vars first (base layer)
	if projectID != nil && *projectID != "" {
		rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM environment_variables WHERE project_id = $1`, *projectID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var k, v string
				if err := rows.Scan(&k, &v); err == nil {
					envVars[k] = v
				}
			}
		}
	}

	// 2. Load service-level env vars (service overrides project)
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM environment_variables WHERE service_id = $1`, serviceID)
	if err != nil {
		return envVars, nil // If table not available or error, return what we have
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			envVars[k] = v
		}
	}

	return envVars, nil
}

// UpdateServiceActiveDeployment updates the active deployment pointer on a service
func (r *PostgresRepository) UpdateServiceActiveDeployment(ctx context.Context, serviceID, deploymentID string) error {
	query := `UPDATE services SET active_deployment_id = $2, updated_at = $3 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, serviceID, deploymentID, time.Now().UTC())
	return err
}

// UpdateServiceReplicas modifies the configured min and max replicas on a service
func (r *PostgresRepository) UpdateServiceReplicas(ctx context.Context, serviceID string, replicas int) error {
	query := `UPDATE services SET min_replicas = $2, max_replicas = $2, updated_at = $3 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, serviceID, replicas, time.Now().UTC())
	return err
}

// MarkPreviousDeploymentsReplaced marks earlier active deployments as replaced
func (r *PostgresRepository) MarkPreviousDeploymentsReplaced(ctx context.Context, serviceID, currentDeploymentID string) error {
	query := `
		UPDATE deployments 
		SET status = 'replaced', updated_at = $3 
		WHERE service_id = $1 AND id != $2 AND status = 'active'
	`
	_, err := r.db.ExecContext(ctx, query, serviceID, currentDeploymentID, time.Now().UTC())
	return err
}

// GetEnvironmentVariablesDetailed loads runtime environment variables, separating non-secret variables from secret keys
func (r *PostgresRepository) GetEnvironmentVariablesDetailed(ctx context.Context, serviceID string, projectID *string) (map[string]string, []string, error) {
	nonSecret := make(map[string]string)
	secretKeyMap := make(map[string]bool)

	// 1. Load project-level variables
	if projectID != nil && *projectID != "" {
		rows, err := r.db.QueryContext(ctx, `SELECT key, value, is_secret FROM environment_variables WHERE project_id = $1 AND scope IN ('runtime', 'all')`, *projectID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var k, v string
				var isSecret bool
				if err := rows.Scan(&k, &v, &isSecret); err == nil {
					if isSecret {
						secretKeyMap[k] = true
						delete(nonSecret, k)
					} else {
						nonSecret[k] = v
						delete(secretKeyMap, k)
					}
				}
			}
		}
	}

	// 2. Load service-level variables (overrides project)
	rows, err := r.db.QueryContext(ctx, `SELECT key, value, is_secret FROM environment_variables WHERE service_id = $1 AND scope IN ('runtime', 'all')`, serviceID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			var isSecret bool
			if err := rows.Scan(&k, &v, &isSecret); err == nil {
				if isSecret {
					secretKeyMap[k] = true
					delete(nonSecret, k)
				} else {
					nonSecret[k] = v
					delete(secretKeyMap, k)
				}
			}
		}
	}

	secretKeys := make([]string, 0, len(secretKeyMap))
	for k := range secretKeyMap {
		secretKeys = append(secretKeys, k)
	}
	sort.Strings(secretKeys)

	return nonSecret, secretKeys, nil
}

// CreateDeploymentEnvSnapshot stores an immutable snapshot of configuration for a deployment
func (r *PostgresRepository) CreateDeploymentEnvSnapshot(ctx context.Context, snap *types.DeploymentEnvSnapshot) error {
	varsJSON, err := json.Marshal(snap.Variables)
	if err != nil {
		varsJSON = []byte("{}")
	}
	secretsJSON, err := json.Marshal(snap.SecretKeys)
	if err != nil {
		secretsJSON = []byte("[]")
	}

	query := `
		INSERT INTO deployment_env_snapshots (id, deployment_id, service_id, config_release_num, variables, secret_keys, vault_snapshot_path, target_env, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (deployment_id) DO UPDATE SET
			variables = EXCLUDED.variables,
			secret_keys = EXCLUDED.secret_keys,
			vault_snapshot_path = EXCLUDED.vault_snapshot_path,
			target_env = EXCLUDED.target_env
	`
	if snap.ID == "" {
		snap.ID = uuid.New().String()
	}
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = time.Now().UTC()
	}
	targetEnv := string(snap.TargetEnv)
	if targetEnv == "" {
		targetEnv = "production"
	}

	var vaultPath sql.NullString
	if snap.VaultSnapshotPath != "" {
		vaultPath = sql.NullString{String: snap.VaultSnapshotPath, Valid: true}
	}

	_, err = r.db.ExecContext(ctx, query, snap.ID, snap.DeploymentID, snap.ServiceID, snap.ConfigReleaseNum, varsJSON, secretsJSON, vaultPath, targetEnv, snap.CreatedAt)
	return err
}

// GetDeploymentEnvSnapshot retrieves the immutable snapshot of a deployment's environment
func (r *PostgresRepository) GetDeploymentEnvSnapshot(ctx context.Context, deploymentID string) (*types.DeploymentEnvSnapshot, error) {
	query := `
		SELECT id, deployment_id, service_id, config_release_num, variables, secret_keys, vault_snapshot_path, target_env, created_at
		FROM deployment_env_snapshots
		WHERE deployment_id = $1
	`
	var s types.DeploymentEnvSnapshot
	var varsJSON, secretsJSON []byte
	var vaultPath sql.NullString
	var targetEnv string

	err := r.db.QueryRowContext(ctx, query, deploymentID).Scan(
		&s.ID, &s.DeploymentID, &s.ServiceID, &s.ConfigReleaseNum, &varsJSON, &secretsJSON, &vaultPath, &targetEnv, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to query deployment snapshot: %w", err)
	}

	_ = json.Unmarshal(varsJSON, &s.Variables)
	_ = json.Unmarshal(secretsJSON, &s.SecretKeys)
	if vaultPath.Valid {
		s.VaultSnapshotPath = vaultPath.String
	}
	s.TargetEnv = types.EnvVarTargetEnv(targetEnv)
	return &s, nil
}

