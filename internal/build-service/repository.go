package buildservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrNotFound       = errors.New("resource not found")
	ErrForbidden      = errors.New("permission denied")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrConflict       = errors.New("resource already exists")
	ErrInvalidRequest = errors.New("invalid request parameters")
	ErrBuildActive    = errors.New("build is already in progress")
	ErrBuildFinished  = errors.New("cannot cancel a completed build")
)

// Repository defines data access methods for build-service
type Repository interface {
	CheckOrgMembership(ctx context.Context, orgID, userID string) (string, error)
	GetServiceByID(ctx context.Context, serviceID string) (*types.Service, error)
	GetOrganizationSlug(ctx context.Context, orgID string) (string, error)

	CreateBuild(ctx context.Context, b *types.Build) error
	GetBuildByID(ctx context.Context, buildID string) (*types.Build, error)
	ListBuildsByService(ctx context.Context, serviceID string, limit, offset int, status string) ([]types.Build, int, error)
	UpdateBuildStatus(ctx context.Context, buildID string, status types.BuildStatus, imageTag, imageDigest, errorMessage string, durationMs int64, finishedAt *time.Time) error

	InsertBuildLog(ctx context.Context, entry *types.BuildLogEntry) error
	InsertBuildLogsBatch(ctx context.Context, entries []types.BuildLogEntry) error
	GetBuildLogs(ctx context.Context, buildID string, limit, offset int) ([]types.BuildLogEntry, error)

	CreateDeployment(ctx context.Context, d *types.Deployment) error
	GetServiceEnvVars(ctx context.Context, serviceID string) (map[string]string, error)
	GetBuildEnvironmentVariables(ctx context.Context, serviceID string, projectID *string) (map[string]string, error)
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a new PostgresRepository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// CheckOrgMembership verifies that a user belongs to the target organization
func (r *PostgresRepository) CheckOrgMembership(ctx context.Context, orgID, userID string) (string, error) {
	query := `SELECT role FROM organization_members WHERE organization_id = $1 AND user_id = $2`
	var role string
	err := r.db.QueryRowContext(ctx, query, orgID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrForbidden
		}
		return "", fmt.Errorf("failed to query organization membership: %w", err)
	}
	return role, nil
}

// GetServiceByID retrieves a service by ID
func (r *PostgresRepository) GetServiceByID(ctx context.Context, serviceID string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       repo_url, branch, COALESCE(root_directory, ''), COALESCE(build_command, ''),
		       COALESCE(start_command, ''), COALESCE(dockerfile_path, ''), port,
		       min_replicas, max_replicas, resources, health_check,
		       COALESCE(webhook_secret, ''), created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var (
		s           types.Service
		projectID   sql.NullString
		resources   []byte
		healthCheck []byte
	)

	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&s.ID,
		&s.OrganizationID,
		&projectID,
		&s.Name,
		&s.Slug,
		&s.Type,
		&s.RuntimeTier,
		&s.RepoURL,
		&s.Branch,
		&s.RootDirectory,
		&s.BuildCommand,
		&s.StartCommand,
		&s.DockerfilePath,
		&s.Port,
		&s.MinReplicas,
		&s.MaxReplicas,
		&resources,
		&healthCheck,
		&s.WebhookSecret,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	if projectID.Valid {
		pID := projectID.String
		s.ProjectID = &pID
	}

	if len(resources) > 0 {
		_ = json.Unmarshal(resources, &s.Resources)
	}
	if len(healthCheck) > 0 {
		_ = json.Unmarshal(healthCheck, &s.HealthCheck)
	}

	return &s, nil
}

// GetOrganizationSlug retrieves the slug for an organization
func (r *PostgresRepository) GetOrganizationSlug(ctx context.Context, orgID string) (string, error) {
	query := `SELECT slug FROM organizations WHERE id = $1`
	var slug string
	err := r.db.QueryRowContext(ctx, query, orgID).Scan(&slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "default", nil
		}
		return "", fmt.Errorf("failed to get organization slug: %w", err)
	}
	return slug, nil
}

// CreateBuild inserts a new build record into the builds table
func (r *PostgresRepository) CreateBuild(ctx context.Context, b *types.Build) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO builds (
			id, service_id, commit_hash, branch, message, author,
			status, image_tag, image_digest, duration_ms, triggered_by,
			error_message, created_at, finished_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query,
		b.ID,
		b.ServiceID,
		b.CommitHash,
		b.Branch,
		b.Message,
		b.Author,
		string(b.Status),
		b.ImageTag,
		b.ImageDigest,
		b.DurationMs,
		b.TriggeredBy,
		b.ErrorMessage,
		b.CreatedAt,
		b.FinishedAt,
	).Scan(&b.ID, &b.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to insert build record: %w", err)
	}
	return nil
}

// GetBuildByID retrieves a single build by its ID
func (r *PostgresRepository) GetBuildByID(ctx context.Context, buildID string) (*types.Build, error) {
	query := `
		SELECT id, service_id, commit_hash, branch, COALESCE(message, ''),
		       COALESCE(author, ''), status, COALESCE(image_tag, ''),
		       COALESCE(image_digest, ''), COALESCE(duration_ms, 0),
		       triggered_by, COALESCE(error_message, ''), created_at, finished_at
		FROM builds
		WHERE id = $1
	`
	var (
		b          types.Build
		statusStr  string
		finishedAt sql.NullTime
	)

	err := r.db.QueryRowContext(ctx, query, buildID).Scan(
		&b.ID,
		&b.ServiceID,
		&b.CommitHash,
		&b.Branch,
		&b.Message,
		&b.Author,
		&statusStr,
		&b.ImageTag,
		&b.ImageDigest,
		&b.DurationMs,
		&b.TriggeredBy,
		&b.ErrorMessage,
		&b.CreatedAt,
		&finishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get build: %w", err)
	}

	b.Status = types.BuildStatus(statusStr)
	if finishedAt.Valid {
		b.FinishedAt = &finishedAt.Time
	}

	return &b, nil
}

// ListBuildsByService returns paginated builds for a given service
func (r *PostgresRepository) ListBuildsByService(ctx context.Context, serviceID string, limit, offset int, status string) ([]types.Build, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// 1. Total count
	countQuery := `SELECT count(*) FROM builds WHERE service_id = $1`
	var countArgs []any
	countArgs = append(countArgs, serviceID)

	if status != "" {
		countQuery += ` AND status = $2`
		countArgs = append(countArgs, status)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count builds: %w", err)
	}

	// 2. Fetch rows
	query := `
		SELECT id, service_id, commit_hash, branch, COALESCE(message, ''),
		       COALESCE(author, ''), status, COALESCE(image_tag, ''),
		       COALESCE(image_digest, ''), COALESCE(duration_ms, 0),
		       triggered_by, COALESCE(error_message, ''), created_at, finished_at
		FROM builds
		WHERE service_id = $1
	`
	var args []any
	args = append(args, serviceID)

	if status != "" {
		query += fmt.Sprintf(` AND status = $%d`, len(args)+1)
		args = append(args, status)
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query builds: %w", err)
	}
	defer rows.Close()

	builds := make([]types.Build, 0)
	for rows.Next() {
		var (
			b          types.Build
			statusStr  string
			finishedAt sql.NullTime
		)
		err := rows.Scan(
			&b.ID,
			&b.ServiceID,
			&b.CommitHash,
			&b.Branch,
			&b.Message,
			&b.Author,
			&statusStr,
			&b.ImageTag,
			&b.ImageDigest,
			&b.DurationMs,
			&b.TriggeredBy,
			&b.ErrorMessage,
			&b.CreatedAt,
			&finishedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan build row: %w", err)
		}
		b.Status = types.BuildStatus(statusStr)
		if finishedAt.Valid {
			b.FinishedAt = &finishedAt.Time
		}
		builds = append(builds, b)
	}

	return builds, total, nil
}

// UpdateBuildStatus updates status, image digests, metrics, and timestamps
func (r *PostgresRepository) UpdateBuildStatus(ctx context.Context, buildID string, status types.BuildStatus, imageTag, imageDigest, errorMessage string, durationMs int64, finishedAt *time.Time) error {
	query := `
		UPDATE builds
		SET status = $2,
		    image_tag = CASE WHEN $3 <> '' THEN $3 ELSE image_tag END,
		    image_digest = CASE WHEN $4 <> '' THEN $4 ELSE image_digest END,
		    error_message = CASE WHEN $5 <> '' THEN $5 ELSE error_message END,
		    duration_ms = CASE WHEN $6 > 0 THEN $6 ELSE duration_ms END,
		    finished_at = $7
		WHERE id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		buildID,
		string(status),
		imageTag,
		imageDigest,
		errorMessage,
		durationMs,
		finishedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update build status: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertBuildLog adds a single log line into build_logs
func (r *PostgresRepository) InsertBuildLog(ctx context.Context, entry *types.BuildLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	if entry.Stream == "" {
		entry.Stream = "stdout"
	}

	query := `
		INSERT INTO build_logs (id, build_id, line, stream, timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, entry.ID, entry.BuildID, entry.Line, entry.Stream, entry.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert build log: %w", err)
	}
	return nil
}

// InsertBuildLogsBatch adds multiple log lines in a transaction
func (r *PostgresRepository) InsertBuildLogsBatch(ctx context.Context, entries []types.BuildLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin log tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO build_logs (id, build_id, line, stream, timestamp) VALUES ($1, $2, $3, $4, $5)`)
	if err != nil {
		return fmt.Errorf("failed to prepare log statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, e := range entries {
		id := e.ID
		if id == "" {
			id = uuid.New().String()
		}
		ts := e.Timestamp
		if ts.IsZero() {
			ts = now
		}
		stream := e.Stream
		if stream == "" {
			stream = "stdout"
		}
		if _, err := stmt.ExecContext(ctx, id, e.BuildID, e.Line, stream, ts); err != nil {
			return fmt.Errorf("failed to insert batch log entry: %w", err)
		}
	}

	return tx.Commit()
}

// GetBuildLogs retrieves log entries for a build
func (r *PostgresRepository) GetBuildLogs(ctx context.Context, buildID string, limit, offset int) ([]types.BuildLogEntry, error) {
	if limit <= 0 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, build_id, line, stream, timestamp
		FROM build_logs
		WHERE build_id = $1
		ORDER BY timestamp ASC, id ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, buildID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query build logs: %w", err)
	}
	defer rows.Close()

	entries := make([]types.BuildLogEntry, 0)
	for rows.Next() {
		var e types.BuildLogEntry
		if err := rows.Scan(&e.ID, &e.BuildID, &e.Line, &e.Stream, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan build log row: %w", err)
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// CreateDeployment records a pending deployment record for Orchestration Service handoff
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

	query := `
		INSERT INTO deployments (id, service_id, build_id, status, nomad_job_id, active_url, replicas, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query,
		d.ID,
		d.ServiceID,
		d.BuildID,
		string(d.Status),
		d.NomadJobID,
		d.ActiveURL,
		d.Replicas,
		d.ErrorMessage,
		d.CreatedAt,
		d.UpdatedAt,
	).Scan(&d.ID, &d.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create deployment record: %w", err)
	}
	return nil
}

// GetServiceEnvVars fetches environment variables configured for a service
func (r *PostgresRepository) GetServiceEnvVars(ctx context.Context, serviceID string) (map[string]string, error) {
	query := `
		SELECT key, value
		FROM environment_variables
		WHERE service_id = $1
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query environment variables: %w", err)
	}
	defer rows.Close()

	envVars := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("failed to scan environment variable: %w", err)
		}
		envVars[k] = v
	}
	return envVars, nil
}

// GetBuildEnvironmentVariables loads project + service env vars filtered for build-time (scope IN ('build', 'all'))
func (r *PostgresRepository) GetBuildEnvironmentVariables(ctx context.Context, serviceID string, projectID *string) (map[string]string, error) {
	envVars := make(map[string]string)

	// 1. Load project-level build vars
	if projectID != nil && *projectID != "" {
		pQuery := `
			SELECT key, value
			FROM environment_variables
			WHERE project_id = $1 AND scope IN ('build', 'all')
		`
		pRows, err := r.db.QueryContext(ctx, pQuery, *projectID)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var k, v string
				if err := pRows.Scan(&k, &v); err == nil {
					envVars[k] = v
				}
			}
		}
	}

	// 2. Load service-level build vars (overrides project)
	sQuery := `
		SELECT key, value
		FROM environment_variables
		WHERE service_id = $1 AND scope IN ('build', 'all')
	`
	sRows, err := r.db.QueryContext(ctx, sQuery, serviceID)
	if err != nil {
		return envVars, nil
	}
	defer sRows.Close()
	for sRows.Next() {
		var k, v string
		if err := sRows.Scan(&k, &v); err == nil {
			envVars[k] = v
		}
	}

	return envVars, nil
}

