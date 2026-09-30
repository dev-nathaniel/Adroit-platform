package gitintegrationservice

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
	ErrConflict       = errors.New("resource already exists")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("permission denied")
	ErrInvalidRequest = errors.New("invalid request parameters")
)

// Repository defines data access operations for git-integration-service
type Repository interface {
	FindMatchingServices(ctx context.Context, repoCanonical, branch string) ([]types.Service, error)
	GetServiceByID(ctx context.Context, serviceID string) (*types.Service, error)
	UpdateServiceWebhookSecret(ctx context.Context, serviceID, secret string) error
	CreateBuild(ctx context.Context, build *types.Build) error

	SaveGitProvider(ctx context.Context, conn *types.GitProviderConnection, accessToken string) error
	GetGitProvider(ctx context.Context, userID string, provider types.GitProviderType) (*types.GitProviderConnection, string, error)
	ListGitProviders(ctx context.Context, userID string) ([]types.GitProviderConnection, error)
	DeleteGitProvider(ctx context.Context, userID string, provider types.GitProviderType) error

	RecordWebhookEvent(ctx context.Context, event *types.GitWebhookEventRecord) error
	ListWebhookEvents(ctx context.Context, limit int) ([]types.GitWebhookEventRecord, error)
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository initializes a Postgres repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// FindMatchingServices retrieves all services that match the repository and branch
func (r *PostgresRepository) FindMatchingServices(ctx context.Context, repoCanonical, branch string) ([]types.Service, error) {
	// Query services where branch matches or branch is blank/main
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       repo_url, branch, root_directory, build_command, start_command,
		       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
		       memory_mb, disk_mb, health_check_path, webhook_secret, created_at, updated_at
		FROM services
		WHERE branch = $1 OR branch = '' OR branch = 'main' OR branch = 'master'
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to query candidate services: %w", err)
	}
	defer rows.Close()

	var matched []types.Service
	for rows.Next() {
		var s types.Service
		var projID, rootDir, buildCmd, startCmd, dockerPath, healthPath, webhookSec sql.NullString
		var sType, rTier string

		err := rows.Scan(
			&s.ID,
			&s.OrganizationID,
			&projID,
			&s.Name,
			&s.Slug,
			&sType,
			&rTier,
			&s.RepoURL,
			&s.Branch,
			&rootDir,
			&buildCmd,
			&startCmd,
			&dockerPath,
			&s.Port,
			&s.MinReplicas,
			&s.MaxReplicas,
			&s.Resources.CPUCores,
			&s.Resources.MemoryMB,
			&s.Resources.DiskMB,
			&healthPath,
			&webhookSec,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
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
		if webhookSec.Valid {
			s.WebhookSecret = webhookSec.String
		}

		// Verify repo canonical match
		if GitURLsMatch(s.RepoURL, repoCanonical) {
			// Branch must match exactly unless service specifies empty/any branch
			if s.Branch == "" || s.Branch == branch {
				matched = append(matched, s)
			}
		}
	}

	return matched, rows.Err()
}

// GetServiceByID retrieves a single service
func (r *PostgresRepository) GetServiceByID(ctx context.Context, serviceID string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       repo_url, branch, root_directory, build_command, start_command,
		       dockerfile_path, port, min_replicas, max_replicas, cpu_cores,
		       memory_mb, disk_mb, health_check_path, webhook_secret, created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var s types.Service
	var projID, rootDir, buildCmd, startCmd, dockerPath, healthPath, webhookSec sql.NullString
	var sType, rTier string

	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&s.ID,
		&s.OrganizationID,
		&projID,
		&s.Name,
		&s.Slug,
		&sType,
		&rTier,
		&s.RepoURL,
		&s.Branch,
		&rootDir,
		&buildCmd,
		&startCmd,
		&dockerPath,
		&s.Port,
		&s.MinReplicas,
		&s.MaxReplicas,
		&s.Resources.CPUCores,
		&s.Resources.MemoryMB,
		&s.Resources.DiskMB,
		&healthPath,
		&webhookSec,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
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
	if webhookSec.Valid {
		s.WebhookSecret = webhookSec.String
	}

	return &s, nil
}

// UpdateServiceWebhookSecret updates the webhook secret for a service
func (r *PostgresRepository) UpdateServiceWebhookSecret(ctx context.Context, serviceID, secret string) error {
	query := `
		UPDATE services
		SET webhook_secret = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`
	res, err := r.db.ExecContext(ctx, query, serviceID, secret)
	if err != nil {
		return fmt.Errorf("failed to update service webhook secret: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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
		INSERT INTO builds (id, service_id, commit_hash, branch, message, author, status, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query,
		b.ID,
		b.ServiceID,
		b.CommitHash,
		b.Branch,
		b.Message,
		b.Author,
		b.Status,
		b.TriggeredBy,
		b.CreatedAt,
	).Scan(&b.ID, &b.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create build record: %w", err)
	}
	return nil
}

// SaveGitProvider saves or updates a user's connected git provider account
func (r *PostgresRepository) SaveGitProvider(ctx context.Context, conn *types.GitProviderConnection, accessToken string) error {
	if conn.ID == "" {
		conn.ID = uuid.New().String()
	}

	query := `
		INSERT INTO git_providers (id, user_id, provider, username, display_name, avatar_url, access_token, base_url, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, provider) DO UPDATE SET
			username = EXCLUDED.username,
			display_name = EXCLUDED.display_name,
			avatar_url = EXCLUDED.avatar_url,
			access_token = EXCLUDED.access_token,
			base_url = EXCLUDED.base_url,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		conn.ID,
		conn.UserID,
		string(conn.Provider),
		conn.Username,
		conn.DisplayName,
		conn.AvatarURL,
		accessToken,
		conn.BaseURL,
	).Scan(&conn.ID, &conn.CreatedAt, &conn.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to save git provider: %w", err)
	}
	return nil
}

// GetGitProvider retrieves provider connection details and access token for a user
func (r *PostgresRepository) GetGitProvider(ctx context.Context, userID string, provider types.GitProviderType) (*types.GitProviderConnection, string, error) {
	// 1. Check dedicated git_providers table
	query := `
		SELECT id, user_id, provider, username, display_name, avatar_url, access_token, base_url, created_at, updated_at
		FROM git_providers
		WHERE user_id = $1 AND provider = $2
	`
	var conn types.GitProviderConnection
	var pStr, token string
	var dName, aURL, bURL sql.NullString

	err := r.db.QueryRowContext(ctx, query, userID, string(provider)).Scan(
		&conn.ID,
		&conn.UserID,
		&pStr,
		&conn.Username,
		&dName,
		&aURL,
		&token,
		&bURL,
		&conn.CreatedAt,
		&conn.UpdatedAt,
	)

	if err == nil {
		conn.Provider = types.GitProviderType(pStr)
		if dName.Valid {
			conn.DisplayName = dName.String
		}
		if aURL.Valid {
			conn.AvatarURL = aURL.String
		}
		if bURL.Valid {
			conn.BaseURL = bURL.String
		}
		return &conn, token, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("failed to query git_providers: %w", err)
	}

	// 2. Fallback to users table for OAuth tokens
	if provider == types.GitProviderGitHub {
		var ghUser, ghToken sql.NullString
		err := r.db.QueryRowContext(ctx, "SELECT github_username, github_access_token FROM users WHERE id = $1", userID).Scan(&ghUser, &ghToken)
		if err == nil && ghToken.Valid && ghToken.String != "" {
			uName := ""
			if ghUser.Valid {
				uName = ghUser.String
			}
			return &types.GitProviderConnection{
				ID:          "oauth_user_" + userID,
				UserID:      userID,
				Provider:    types.GitProviderGitHub,
				Username:    uName,
				DisplayName: uName,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}, ghToken.String, nil
		}
	} else if provider == types.GitProviderGitLab {
		var glUser, glToken sql.NullString
		err := r.db.QueryRowContext(ctx, "SELECT gitlab_username, gitlab_access_token FROM users WHERE id = $1", userID).Scan(&glUser, &glToken)
		if err == nil && glToken.Valid && glToken.String != "" {
			uName := ""
			if glUser.Valid {
				uName = glUser.String
			}
			return &types.GitProviderConnection{
				ID:          "oauth_user_" + userID,
				UserID:      userID,
				Provider:    types.GitProviderGitLab,
				Username:    uName,
				DisplayName: uName,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}, glToken.String, nil
		}
	}

	return nil, "", ErrNotFound
}

// ListGitProviders returns all connected git providers for a user
func (r *PostgresRepository) ListGitProviders(ctx context.Context, userID string) ([]types.GitProviderConnection, error) {
	query := `
		SELECT id, user_id, provider, username, display_name, avatar_url, base_url, created_at, updated_at
		FROM git_providers
		WHERE user_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query git_providers: %w", err)
	}
	defer rows.Close()

	var list []types.GitProviderConnection
	foundProviders := make(map[string]bool)

	for rows.Next() {
		var conn types.GitProviderConnection
		var pStr string
		var dName, aURL, bURL sql.NullString

		if err := rows.Scan(
			&conn.ID,
			&conn.UserID,
			&pStr,
			&conn.Username,
			&dName,
			&aURL,
			&bURL,
			&conn.CreatedAt,
			&conn.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan git_provider: %w", err)
		}
		conn.Provider = types.GitProviderType(pStr)
		if dName.Valid {
			conn.DisplayName = dName.String
		}
		if aURL.Valid {
			conn.AvatarURL = aURL.String
		}
		if bURL.Valid {
			conn.BaseURL = bURL.String
		}
		list = append(list, conn)
		foundProviders[string(conn.Provider)] = true
	}

	// Check user table fallback if GitHub not yet in list
	if !foundProviders[string(types.GitProviderGitHub)] {
		var ghUser, ghToken sql.NullString
		_ = r.db.QueryRowContext(ctx, "SELECT github_username, github_access_token FROM users WHERE id = $1", userID).Scan(&ghUser, &ghToken)
		if ghToken.Valid && ghToken.String != "" {
			uName := ""
			if ghUser.Valid {
				uName = ghUser.String
			}
			list = append(list, types.GitProviderConnection{
				ID:          "oauth_github_" + userID,
				UserID:      userID,
				Provider:    types.GitProviderGitHub,
				Username:    uName,
				DisplayName: uName,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			})
		}
	}

	return list, rows.Err()
}

// DeleteGitProvider removes a provider integration for a user
func (r *PostgresRepository) DeleteGitProvider(ctx context.Context, userID string, provider types.GitProviderType) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM git_providers WHERE user_id = $1 AND provider = $2", userID, string(provider))
	if err != nil {
		return fmt.Errorf("failed to delete git provider: %w", err)
	}

	// Also clear token from user table if matching
	if provider == types.GitProviderGitHub {
		_, _ = r.db.ExecContext(ctx, "UPDATE users SET github_access_token = NULL WHERE id = $1", userID)
	} else if provider == types.GitProviderGitLab {
		_, _ = r.db.ExecContext(ctx, "UPDATE users SET gitlab_access_token = NULL WHERE id = $1", userID)
	}

	return nil
}

// RecordWebhookEvent persists an incoming webhook execution log
func (r *PostgresRepository) RecordWebhookEvent(ctx context.Context, ev *types.GitWebhookEventRecord) error {
	if ev.ID == "" {
		ev.ID = uuid.New().String()
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}

	matchedJSON, _ := json.Marshal(ev.MatchedServices)
	triggeredJSON, _ := json.Marshal(ev.TriggeredBuilds)

	query := `
		INSERT INTO git_webhook_events (
			id, provider, event_type, delivery_id, repo_url, branch,
			commit_hash, commit_message, author, status, error_message,
			matched_services, triggered_builds, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.db.ExecContext(ctx, query,
		ev.ID,
		ev.Provider,
		ev.EventType,
		ev.DeliveryID,
		ev.RepoURL,
		ev.Branch,
		ev.CommitHash,
		ev.CommitMessage,
		ev.Author,
		ev.Status,
		ev.ErrorMessage,
		matchedJSON,
		triggeredJSON,
		ev.CreatedAt,
	)
	return err
}

// ListWebhookEvents returns recent webhook events for auditing
func (r *PostgresRepository) ListWebhookEvents(ctx context.Context, limit int) ([]types.GitWebhookEventRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT id, provider, event_type, delivery_id, repo_url, branch,
		       commit_hash, commit_message, author, status, error_message,
		       matched_services, triggered_builds, created_at
		FROM git_webhook_events
		ORDER BY created_at DESC
		LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhook events: %w", err)
	}
	defer rows.Close()

	var events []types.GitWebhookEventRecord
	for rows.Next() {
		var ev types.GitWebhookEventRecord
		var delID, branch, cHash, cMsg, author, errMsg sql.NullString
		var matchedRaw, triggeredRaw []byte

		err := rows.Scan(
			&ev.ID,
			&ev.Provider,
			&ev.EventType,
			&delID,
			&ev.RepoURL,
			&branch,
			&cHash,
			&cMsg,
			&author,
			&ev.Status,
			&errMsg,
			&matchedRaw,
			&triggeredRaw,
			&ev.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan webhook event: %w", err)
		}

		if delID.Valid {
			ev.DeliveryID = delID.String
		}
		if branch.Valid {
			ev.Branch = branch.String
		}
		if cHash.Valid {
			ev.CommitHash = cHash.String
		}
		if cMsg.Valid {
			ev.CommitMessage = cMsg.String
		}
		if author.Valid {
			ev.Author = author.String
		}
		if errMsg.Valid {
			ev.ErrorMessage = errMsg.String
		}

		_ = json.Unmarshal(matchedRaw, &ev.MatchedServices)
		_ = json.Unmarshal(triggeredRaw, &ev.TriggeredBuilds)

		events = append(events, ev)
	}

	return events, rows.Err()
}
