package logservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrLogNotFound     = errors.New("log entry not found")
	ErrServiceNotFound = errors.New("service not found")
	ErrUnauthorized    = errors.New("unauthorized access to service logs")
)

// Repository defines storage operations for application and container runtime logs
type Repository interface {
	InsertLog(ctx context.Context, entry types.AppLogEntry) error
	InsertLogsBatch(ctx context.Context, entries []types.AppLogEntry) error
	GetLogs(ctx context.Context, filter types.AppLogsQueryFilter) ([]types.AppLogEntry, int, error)
	GetDeploymentLogs(ctx context.Context, deploymentID string, filter types.AppLogsQueryFilter) ([]types.AppLogEntry, int, error)
	PurgeLogs(ctx context.Context, serviceID string, before time.Time) (int64, error)
	GetService(ctx context.Context, serviceID string) (*types.Service, error)
	UserHasOrgAccess(ctx context.Context, userID, orgID string) (bool, error)
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository initializes a Postgres-backed log repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// InsertLog inserts an individual log entry
func (r *PostgresRepository) InsertLog(ctx context.Context, entry types.AppLogEntry) error {
	if entry.ID == "" {
		entry.ID = "log_" + uuid.New().String()[:12]
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	if entry.Stream == "" {
		entry.Stream = "stdout"
	}

	depID := entry.DeploymentID
	if depID != nil && strings.TrimSpace(*depID) == "" {
		depID = nil
	}
	allocID := entry.AllocationID
	if allocID != nil && strings.TrimSpace(*allocID) == "" {
		allocID = nil
	}

	query := `
		INSERT INTO app_logs (id, service_id, deployment_id, allocation_id, stream, message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query,
		entry.ID,
		entry.ServiceID,
		depID,
		allocID,
		entry.Stream,
		entry.Message,
		entry.Timestamp,
	)
	return err
}

// InsertLogsBatch inserts multiple log records in a single database transaction
func (r *PostgresRepository) InsertLogsBatch(ctx context.Context, entries []types.AppLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO app_logs (id, service_id, deployment_id, allocation_id, stream, message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch insert statement: %w", err)
	}
	defer stmt.Close()

	for _, entry := range entries {
		id := entry.ID
		if id == "" {
			id = "log_" + uuid.New().String()[:12]
		}
		ts := entry.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		stream := entry.Stream
		if stream == "" {
			stream = "stdout"
		}
		depID := entry.DeploymentID
		if depID != nil && strings.TrimSpace(*depID) == "" {
			depID = nil
		}
		allocID := entry.AllocationID
		if allocID != nil && strings.TrimSpace(*allocID) == "" {
			allocID = nil
		}

		_, err := stmt.ExecContext(ctx,
			id,
			entry.ServiceID,
			depID,
			allocID,
			stream,
			entry.Message,
			ts,
		)
		if err != nil {
			return fmt.Errorf("failed to insert log record: %w", err)
		}
	}

	return tx.Commit()
}

// GetLogs queries log entries matching the specified filter parameters
func (r *PostgresRepository) GetLogs(ctx context.Context, filter types.AppLogsQueryFilter) ([]types.AppLogEntry, int, error) {
	whereClauses := []string{"service_id = $1"}
	args := []any{filter.ServiceID}
	argIdx := 2

	if filter.DeploymentID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("deployment_id = $%d", argIdx))
		args = append(args, filter.DeploymentID)
		argIdx++
	}

	if filter.AllocationID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("allocation_id = $%d", argIdx))
		args = append(args, filter.AllocationID)
		argIdx++
	}

	if filter.Stream != "" && filter.Stream != "all" {
		whereClauses = append(whereClauses, fmt.Sprintf("stream = $%d", argIdx))
		args = append(args, filter.Stream)
		argIdx++
	}

	if filter.Since != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp >= $%d", argIdx))
		args = append(args, *filter.Since)
		argIdx++
	}

	if filter.Until != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp <= $%d", argIdx))
		args = append(args, *filter.Until)
		argIdx++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("message ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// 1. Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM app_logs WHERE %s", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	// 2. Fetch records
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, service_id, deployment_id, allocation_id, stream, message, timestamp
		FROM app_logs
		WHERE %s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []types.AppLogEntry
	for rows.Next() {
		var entry types.AppLogEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.ServiceID,
			&entry.DeploymentID,
			&entry.AllocationID,
			&entry.Stream,
			&entry.Message,
			&entry.Timestamp,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan log row: %w", err)
		}
		logs = append(logs, entry)
	}

	// Return chronological order (oldest to newest) for viewer readability
	reverseLogs(logs)

	return logs, total, rows.Err()
}

// GetDeploymentLogs retrieves logs for a specific deployment
func (r *PostgresRepository) GetDeploymentLogs(ctx context.Context, deploymentID string, filter types.AppLogsQueryFilter) ([]types.AppLogEntry, int, error) {
	whereClauses := []string{"deployment_id = $1"}
	args := []any{deploymentID}
	argIdx := 2

	if filter.Stream != "" && filter.Stream != "all" {
		whereClauses = append(whereClauses, fmt.Sprintf("stream = $%d", argIdx))
		args = append(args, filter.Stream)
		argIdx++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("message ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM app_logs WHERE %s", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count deployment logs: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, service_id, deployment_id, allocation_id, stream, message, timestamp
		FROM app_logs
		WHERE %s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query deployment logs: %w", err)
	}
	defer rows.Close()

	var logs []types.AppLogEntry
	for rows.Next() {
		var entry types.AppLogEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.ServiceID,
			&entry.DeploymentID,
			&entry.AllocationID,
			&entry.Stream,
			&entry.Message,
			&entry.Timestamp,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan log row: %w", err)
		}
		logs = append(logs, entry)
	}

	reverseLogs(logs)

	return logs, total, rows.Err()
}

// PurgeLogs deletes log records older than the given cutoff time for a service
func (r *PostgresRepository) PurgeLogs(ctx context.Context, serviceID string, before time.Time) (int64, error) {
	query := `
		DELETE FROM app_logs
		WHERE service_id = $1 AND timestamp < $2
	`
	res, err := r.db.ExecContext(ctx, query, serviceID, before)
	if err != nil {
		return 0, fmt.Errorf("failed to purge logs: %w", err)
	}
	return res.RowsAffected()
}

// GetService fetches service metadata
func (r *PostgresRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	query := `
		SELECT id, organization_id, project_id, name, slug, type, runtime_tier,
		       COALESCE(repo_url, ''), branch, port, min_replicas, max_replicas,
		       created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var s types.Service
	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&s.ID, &s.OrganizationID, &s.ProjectID, &s.Name, &s.Slug, &s.Type, &s.RuntimeTier,
		&s.RepoURL, &s.Branch, &s.Port, &s.MinReplicas, &s.MaxReplicas,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}
	return &s, nil
}

// UserHasOrgAccess verifies if a user is an owner or member of an organization
func (r *PostgresRepository) UserHasOrgAccess(ctx context.Context, userID, orgID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM organizations WHERE id = $1 AND owner_id = $2
			UNION
			SELECT 1 FROM organization_members WHERE organization_id = $1 AND user_id = $2
		)
	`
	var hasAccess bool
	err := r.db.QueryRowContext(ctx, query, orgID, userID).Scan(&hasAccess)
	return hasAccess, err
}

func reverseLogs(logs []types.AppLogEntry) {
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
}
