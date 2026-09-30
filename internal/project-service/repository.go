package projectservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrNotFound       = errors.New("resource not found")
	ErrConflict       = errors.New("resource already exists")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("permission denied")
	ErrInvalidRequest = errors.New("invalid request parameters")
)

// Repository defines the persistence interface for project-service
type Repository interface {
	CheckOrgMembership(ctx context.Context, orgID, userID string) (string, error)

	CreateProject(ctx context.Context, p *types.Project) error
	GetProjectByID(ctx context.Context, id string) (*types.Project, error)
	GetProjectBySlug(ctx context.Context, orgID, slug string) (*types.Project, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]types.Project, error)
	ListProjectsByUser(ctx context.Context, userID string) ([]types.Project, error)
	UpdateProject(ctx context.Context, p *types.Project) error
	DeleteProject(ctx context.Context, id string) error
	CountServicesInProject(ctx context.Context, projectID string) (int, error)

	CreateService(ctx context.Context, s *types.Service) error
	GetServiceByID(ctx context.Context, id string) (*types.Service, error)
	GetServiceBySlug(ctx context.Context, orgID, slug string) (*types.Service, error)
	ListServices(ctx context.Context, orgID string, projectID *string, standaloneOnly bool) ([]types.Service, error)
	UpdateService(ctx context.Context, s *types.Service) error
	MoveService(ctx context.Context, serviceID string, newProjectID *string) error
	DeleteService(ctx context.Context, id string) error

	SetServiceEnvVars(ctx context.Context, orgID, serviceID string, vars []EnvVarItem) error
	GetServiceEnvVars(ctx context.Context, serviceID string) ([]types.EnvironmentVariable, error)
	DeleteServiceEnvVar(ctx context.Context, serviceID, key string) error

	SetProjectEnvVars(ctx context.Context, orgID, projectID string, vars []EnvVarItem) error
	GetProjectEnvVars(ctx context.Context, projectID string) ([]types.EnvironmentVariable, error)
	DeleteProjectEnvVar(ctx context.Context, projectID, key string) error
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a new PostgresRepository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// CheckOrgMembership verifies user is member of organization and returns their role
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

// CreateProject inserts a new project
func (r *PostgresRepository) CreateProject(ctx context.Context, p *types.Project) error {
	query := `
		INSERT INTO projects (id, organization_id, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query, p.ID, p.OrganizationID, p.Name, p.Slug, p.Description, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	return nil
}

// GetProjectByID retrieves project by ID
func (r *PostgresRepository) GetProjectByID(ctx context.Context, id string) (*types.Project, error) {
	query := `
		SELECT id, organization_id, name, slug, description, created_at, updated_at
		FROM projects
		WHERE id = $1
	`
	var p types.Project
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &desc, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query project by id: %w", err)
	}
	if desc.Valid {
		p.Description = desc.String
	}
	return &p, nil
}

// GetProjectBySlug retrieves project by organization ID and slug
func (r *PostgresRepository) GetProjectBySlug(ctx context.Context, orgID, slug string) (*types.Project, error) {
	query := `
		SELECT id, organization_id, name, slug, description, created_at, updated_at
		FROM projects
		WHERE organization_id = $1 AND slug = $2
	`
	var p types.Project
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx, query, orgID, slug).Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &desc, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query project by slug: %w", err)
	}
	if desc.Valid {
		p.Description = desc.String
	}
	return &p, nil
}

// ListProjectsByOrg lists all projects belonging to an organization
func (r *PostgresRepository) ListProjectsByOrg(ctx context.Context, orgID string) ([]types.Project, error) {
	query := `
		SELECT id, organization_id, name, slug, description, created_at, updated_at
		FROM projects
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects by org: %w", err)
	}
	defer rows.Close()

	var projects []types.Project
	for rows.Next() {
		var p types.Project
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &desc, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		if desc.Valid {
			p.Description = desc.String
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ListProjectsByUser lists all projects across all organizations the user belongs to
func (r *PostgresRepository) ListProjectsByUser(ctx context.Context, userID string) ([]types.Project, error) {
	query := `
		SELECT p.id, p.organization_id, p.name, p.slug, p.description, p.created_at, p.updated_at
		FROM projects p
		JOIN organization_members m ON m.organization_id = p.organization_id
		WHERE m.user_id = $1
		ORDER BY p.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects by user: %w", err)
	}
	defer rows.Close()

	var projects []types.Project
	for rows.Next() {
		var p types.Project
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &desc, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		if desc.Valid {
			p.Description = desc.String
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProject updates mutable project fields
func (r *PostgresRepository) UpdateProject(ctx context.Context, p *types.Project) error {
	query := `
		UPDATE projects
		SET name = $1, slug = $2, description = $3, updated_at = $4
		WHERE id = $5
	`
	res, err := r.db.ExecContext(ctx, query, p.Name, p.Slug, p.Description, p.UpdatedAt, p.ID)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProject removes a project (services become standalone due to ON DELETE SET NULL)
func (r *PostgresRepository) DeleteProject(ctx context.Context, id string) error {
	query := `DELETE FROM projects WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CountServicesInProject returns the number of services attached to a project
func (r *PostgresRepository) CountServicesInProject(ctx context.Context, projectID string) (int, error) {
	query := `SELECT COUNT(*) FROM services WHERE project_id = $1`
	var count int
	err := r.db.QueryRowContext(ctx, query, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count services in project: %w", err)
	}
	return count, nil
}

// CreateService inserts a new service record
func (r *PostgresRepository) CreateService(ctx context.Context, s *types.Service) error {
	query := `
		INSERT INTO services (
			id, organization_id, project_id, name, slug, type, runtime_tier,
			repo_url, branch, root_directory, build_command, start_command,
			dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
			memory_mb, disk_mb, health_check_path, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		s.ID, s.OrganizationID, s.ProjectID, s.Name, s.Slug, string(s.Type), string(s.RuntimeTier),
		s.RepoURL, s.Branch, s.RootDirectory, s.BuildCommand, s.StartCommand,
		s.DockerfilePath, s.Port, s.MinReplicas, s.MaxReplicas, s.Resources.CPUCores,
		s.Resources.MemoryMB, s.Resources.DiskMB, s.HealthCheck.Path, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	return nil
}

// GetServiceByID retrieves a service by ID
func (r *PostgresRepository) GetServiceByID(ctx context.Context, id string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       repo_url, branch, root_directory, build_command, start_command,
		       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
		       memory_mb, disk_mb, health_check_path, created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var s types.Service
	var projID, rootDir, buildCmd, startCmd, dockerPath, healthPath sql.NullString
	var sType, rTier string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.OrganizationID, &projID, &s.Name, &s.Slug, &sType, &rTier,
		&s.RepoURL, &s.Branch, &rootDir, &buildCmd, &startCmd,
		&dockerPath, &s.Port, &s.MinReplicas, &s.MaxReplicas, &s.Resources.CPUCores,
		&s.Resources.MemoryMB, &s.Resources.DiskMB, &healthPath, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service by id: %w", err)
	}

	if projID.Valid {
		pID := projID.String
		s.ProjectID = &pID
	}
	s.Type = types.ServiceType(sType)
	s.RuntimeTier = types.RuntimeTier(rTier)
	if rootDir.Valid {
		s.RootDirectory = rootDir.String
	}
	if buildCmd.Valid {
		s.BuildCommand = buildCmd.String
	}
	if startCmd.Valid {
		s.StartCommand = startCmd.String
	}
	if dockerPath.Valid {
		s.DockerfilePath = dockerPath.String
	}
	if healthPath.Valid {
		s.HealthCheck.Path = healthPath.String
	}

	return &s, nil
}

// GetServiceBySlug retrieves a service by organization ID and slug
func (r *PostgresRepository) GetServiceBySlug(ctx context.Context, orgID, slug string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       repo_url, branch, root_directory, build_command, start_command,
		       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
		       memory_mb, disk_mb, health_check_path, created_at, updated_at
		FROM services
		WHERE organization_id = $1 AND slug = $2
	`
	var s types.Service
	var projID, rootDir, buildCmd, startCmd, dockerPath, healthPath sql.NullString
	var sType, rTier string

	err := r.db.QueryRowContext(ctx, query, orgID, slug).Scan(
		&s.ID, &s.OrganizationID, &projID, &s.Name, &s.Slug, &sType, &rTier,
		&s.RepoURL, &s.Branch, &rootDir, &buildCmd, &startCmd,
		&dockerPath, &s.Port, &s.MinReplicas, &s.MaxReplicas, &s.Resources.CPUCores,
		&s.Resources.MemoryMB, &s.Resources.DiskMB, &healthPath, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service by slug: %w", err)
	}

	if projID.Valid {
		pID := projID.String
		s.ProjectID = &pID
	}
	s.Type = types.ServiceType(sType)
	s.RuntimeTier = types.RuntimeTier(rTier)
	if rootDir.Valid {
		s.RootDirectory = rootDir.String
	}
	if buildCmd.Valid {
		s.BuildCommand = buildCmd.String
	}
	if startCmd.Valid {
		s.StartCommand = startCmd.String
	}
	if dockerPath.Valid {
		s.DockerfilePath = dockerPath.String
	}
	if healthPath.Valid {
		s.HealthCheck.Path = healthPath.String
	}

	return &s, nil
}

// ListServices returns services for an organization, optionally filtered by project or standaloneOnly
func (r *PostgresRepository) ListServices(ctx context.Context, orgID string, projectID *string, standaloneOnly bool) ([]types.Service, error) {
	var query string
	var args []any

	if standaloneOnly {
		query = `
			SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
			       repo_url, branch, root_directory, build_command, start_command,
			       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
			       memory_mb, disk_mb, health_check_path, created_at, updated_at
			FROM services
			WHERE organization_id = $1 AND project_id IS NULL
			ORDER BY created_at DESC
		`
		args = []any{orgID}
	} else if projectID != nil && *projectID != "" {
		query = `
			SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
			       repo_url, branch, root_directory, build_command, start_command,
			       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
			       memory_mb, disk_mb, health_check_path, created_at, updated_at
			FROM services
			WHERE organization_id = $1 AND project_id = $2
			ORDER BY created_at DESC
		`
		args = []any{orgID, *projectID}
	} else {
		query = `
			SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
			       repo_url, branch, root_directory, build_command, start_command,
			       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
			       memory_mb, disk_mb, health_check_path, created_at, updated_at
			FROM services
			WHERE organization_id = $1
			ORDER BY created_at DESC
		`
		args = []any{orgID}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query services: %w", err)
	}
	defer rows.Close()

	var services []types.Service
	for rows.Next() {
		var s types.Service
		var projID, rootDir, buildCmd, startCmd, dockerPath, healthPath sql.NullString
		var sType, rTier string

		if err := rows.Scan(
			&s.ID, &s.OrganizationID, &projID, &s.Name, &s.Slug, &sType, &rTier,
			&s.RepoURL, &s.Branch, &rootDir, &buildCmd, &startCmd,
			&dockerPath, &s.Port, &s.MinReplicas, &s.MaxReplicas, &s.Resources.CPUCores,
			&s.Resources.MemoryMB, &s.Resources.DiskMB, &healthPath, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan service: %w", err)
		}

		if projID.Valid {
			pID := projID.String
			s.ProjectID = &pID
		}
		s.Type = types.ServiceType(sType)
		s.RuntimeTier = types.RuntimeTier(rTier)
		if rootDir.Valid {
			s.RootDirectory = rootDir.String
		}
		if buildCmd.Valid {
			s.BuildCommand = buildCmd.String
		}
		if startCmd.Valid {
			s.StartCommand = startCmd.String
		}
		if dockerPath.Valid {
			s.DockerfilePath = dockerPath.String
		}
		if healthPath.Valid {
			s.HealthCheck.Path = healthPath.String
		}

		services = append(services, s)
	}

	return services, rows.Err()
}

// UpdateService modifies mutable fields of an existing service
func (r *PostgresRepository) UpdateService(ctx context.Context, s *types.Service) error {
	query := `
		UPDATE services SET
			name = $1, slug = $2, type = $3, runtime_tier = $4, repo_url = $5,
			branch = $6, root_directory = $7, build_command = $8, start_command = $9,
			dockerfile_path = $10, port = $11, min_replicas = $12, max_replicas = $13,
			cpu_cores = $14, memory_mb = $15, disk_mb = $16, health_check_path = $17,
			updated_at = $18
		WHERE id = $19
	`
	res, err := r.db.ExecContext(ctx, query,
		s.Name, s.Slug, string(s.Type), string(s.RuntimeTier), s.RepoURL,
		s.Branch, s.RootDirectory, s.BuildCommand, s.StartCommand,
		s.DockerfilePath, s.Port, s.MinReplicas, s.MaxReplicas,
		s.Resources.CPUCores, s.Resources.MemoryMB, s.Resources.DiskMB, s.HealthCheck.Path,
		s.UpdatedAt, s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update service: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveService moves a service to another project or sets it to standalone (newProjectID == nil)
func (r *PostgresRepository) MoveService(ctx context.Context, serviceID string, newProjectID *string) error {
	query := `UPDATE services SET project_id = $1, updated_at = $2 WHERE id = $3`
	res, err := r.db.ExecContext(ctx, query, newProjectID, time.Now().UTC(), serviceID)
	if err != nil {
		return fmt.Errorf("failed to move service: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteService removes a service by ID
func (r *PostgresRepository) DeleteService(ctx context.Context, id string) error {
	query := `DELETE FROM services WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetServiceEnvVars upserts environment variables for a service
func (r *PostgresRepository) SetServiceEnvVars(ctx context.Context, orgID, serviceID string, vars []EnvVarItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO environment_variables (id, organization_id, service_id, key, value, is_secret, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (service_id, key) WHERE service_id IS NOT NULL
		DO UPDATE SET value = EXCLUDED.value, is_secret = EXCLUDED.is_secret, updated_at = EXCLUDED.updated_at
	`
	now := time.Now().UTC()
	for _, v := range vars {
		id := uuid.New().String()
		if _, err := tx.ExecContext(ctx, query, id, orgID, serviceID, v.Key, v.Value, v.IsSecret, now, now); err != nil {
			return fmt.Errorf("failed to upsert service env var %s: %w", v.Key, err)
		}
	}

	return tx.Commit()
}

// GetServiceEnvVars returns all environment variables explicitly attached to a service
func (r *PostgresRepository) GetServiceEnvVars(ctx context.Context, serviceID string) ([]types.EnvironmentVariable, error) {
	query := `
		SELECT id, organization_id, project_id, service_id, key, value, is_secret, created_at, updated_at
		FROM environment_variables
		WHERE service_id = $1
		ORDER BY key ASC
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query service env vars: %w", err)
	}
	defer rows.Close()

	var vars []types.EnvironmentVariable
	for rows.Next() {
		var ev types.EnvironmentVariable
		var pID, sID sql.NullString
		if err := rows.Scan(&ev.ID, &ev.OrganizationID, &pID, &sID, &ev.Key, &ev.Value, &ev.IsSecret, &ev.CreatedAt, &ev.UpdatedAt); err != nil {
			return nil, err
		}
		if pID.Valid {
			pid := pID.String
			ev.ProjectID = &pid
		}
		if sID.Valid {
			sid := sID.String
			ev.ServiceID = &sid
		}
		vars = append(vars, ev)
	}
	return vars, rows.Err()
}

// DeleteServiceEnvVar removes an environment variable from a service
func (r *PostgresRepository) DeleteServiceEnvVar(ctx context.Context, serviceID, key string) error {
	query := `DELETE FROM environment_variables WHERE service_id = $1 AND key = $2`
	res, err := r.db.ExecContext(ctx, query, serviceID, key)
	if err != nil {
		return fmt.Errorf("failed to delete service env var: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProjectEnvVars upserts project-level environment variables
func (r *PostgresRepository) SetProjectEnvVars(ctx context.Context, orgID, projectID string, vars []EnvVarItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO environment_variables (id, organization_id, project_id, key, value, is_secret, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, key) WHERE project_id IS NOT NULL
		DO UPDATE SET value = EXCLUDED.value, is_secret = EXCLUDED.is_secret, updated_at = EXCLUDED.updated_at
	`
	now := time.Now().UTC()
	for _, v := range vars {
		id := uuid.New().String()
		if _, err := tx.ExecContext(ctx, query, id, orgID, projectID, v.Key, v.Value, v.IsSecret, now, now); err != nil {
			return fmt.Errorf("failed to upsert project env var %s: %w", v.Key, err)
		}
	}

	return tx.Commit()
}

// GetProjectEnvVars returns all environment variables attached to a project
func (r *PostgresRepository) GetProjectEnvVars(ctx context.Context, projectID string) ([]types.EnvironmentVariable, error) {
	query := `
		SELECT id, organization_id, project_id, service_id, key, value, is_secret, created_at, updated_at
		FROM environment_variables
		WHERE project_id = $1
		ORDER BY key ASC
	`
	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query project env vars: %w", err)
	}
	defer rows.Close()

	var vars []types.EnvironmentVariable
	for rows.Next() {
		var ev types.EnvironmentVariable
		var pID, sID sql.NullString
		if err := rows.Scan(&ev.ID, &ev.OrganizationID, &pID, &sID, &ev.Key, &ev.Value, &ev.IsSecret, &ev.CreatedAt, &ev.UpdatedAt); err != nil {
			return nil, err
		}
		if pID.Valid {
			pid := pID.String
			ev.ProjectID = &pid
		}
		if sID.Valid {
			sid := sID.String
			ev.ServiceID = &sid
		}
		vars = append(vars, ev)
	}
	return vars, rows.Err()
}

// DeleteProjectEnvVar removes an environment variable from a project
func (r *PostgresRepository) DeleteProjectEnvVar(ctx context.Context, projectID, key string) error {
	query := `DELETE FROM environment_variables WHERE project_id = $1 AND key = $2`
	res, err := r.db.ExecContext(ctx, query, projectID, key)
	if err != nil {
		return fmt.Errorf("failed to delete project env var: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
