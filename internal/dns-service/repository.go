package dnsservice

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrDomainNotFound      = errors.New("custom domain not found")
	ErrDomainAlreadyExists = errors.New("custom domain is already registered")
	ErrRouteNotFound       = errors.New("ingress route not found")
	ErrServiceNotFound     = errors.New("service not found")
)

// Repository defines all database interactions for custom domains and ingress routes
type Repository interface {
	CreateDomain(ctx context.Context, d *types.CustomDomain) error
	GetDomainByID(ctx context.Context, id string) (*types.CustomDomain, error)
	GetDomainByName(ctx context.Context, domain string) (*types.CustomDomain, error)
	ListDomainsByService(ctx context.Context, serviceID string, limit, offset int) ([]types.CustomDomain, int, error)
	UpdateDomainStatus(ctx context.Context, id string, status types.DomainStatus, sslStatus types.SSLStatus, sslError string, verifiedAt *time.Time) error
	DeleteDomain(ctx context.Context, id string) error
	SetPrimaryDomain(ctx context.Context, serviceID, domainID string) error

	UpsertIngressRoute(ctx context.Context, route *types.IngressRoute) error
	GetIngressRouteByHost(ctx context.Context, hostPattern string) (*types.IngressRoute, error)
	ListIngressRoutesByService(ctx context.Context, serviceID string) ([]types.IngressRoute, error)
	ListAllIngressRoutes(ctx context.Context, limit, offset int) ([]types.IngressRoute, int, error)
	DeleteIngressRoute(ctx context.Context, hostPattern string) error

	GetService(ctx context.Context, serviceID string) (*types.Service, error)
	GetActiveDeploymentForService(ctx context.Context, serviceID string) (*types.Deployment, error)
}

// PostgresRepository implements Repository backed by PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository initializes a Postgres-backed repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateDomain(ctx context.Context, d *types.CustomDomain) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = types.DomainStatusPendingVerification
	}
	if d.SSLStatus == "" {
		d.SSLStatus = types.SSLStatusPending
	}
	if d.VerificationType == "" {
		d.VerificationType = types.VerificationTypeCNAME
	}
	d.Domain = strings.ToLower(strings.TrimSpace(d.Domain))

	query := `
		INSERT INTO custom_domains (
			id, service_id, domain, status, verification_type, verification_token,
			is_primary, ssl_status, ssl_error, verified_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(
		ctx, query,
		d.ID, d.ServiceID, d.Domain, d.Status, d.VerificationType, d.VerificationToken,
		d.IsPrimary, d.SSLStatus, d.SSLError, d.VerifiedAt, d.CreatedAt, d.UpdatedAt,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "uq_custom_domains_domain") {
			return ErrDomainAlreadyExists
		}
		return err
	}
	return nil
}

func (r *PostgresRepository) GetDomainByID(ctx context.Context, id string) (*types.CustomDomain, error) {
	query := `
		SELECT id, service_id, domain, status, verification_type, verification_token,
		       is_primary, ssl_status, COALESCE(ssl_error, ''), verified_at, created_at, updated_at
		FROM custom_domains
		WHERE id = $1
	`
	var d types.CustomDomain
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&d.ID, &d.ServiceID, &d.Domain, &d.Status, &d.VerificationType, &d.VerificationToken,
		&d.IsPrimary, &d.SSLStatus, &d.SSLError, &d.VerifiedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDomainNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *PostgresRepository) GetDomainByName(ctx context.Context, domain string) (*types.CustomDomain, error) {
	query := `
		SELECT id, service_id, domain, status, verification_type, verification_token,
		       is_primary, ssl_status, COALESCE(ssl_error, ''), verified_at, created_at, updated_at
		FROM custom_domains
		WHERE domain = $1
	`
	var d types.CustomDomain
	cleanDomain := strings.ToLower(strings.TrimSpace(domain))
	err := r.db.QueryRowContext(ctx, query, cleanDomain).Scan(
		&d.ID, &d.ServiceID, &d.Domain, &d.Status, &d.VerificationType, &d.VerificationToken,
		&d.IsPrimary, &d.SSLStatus, &d.SSLError, &d.VerifiedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDomainNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *PostgresRepository) ListDomainsByService(ctx context.Context, serviceID string, limit, offset int) ([]types.CustomDomain, int, error) {
	countQuery := `SELECT COUNT(*) FROM custom_domains WHERE service_id = $1`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, serviceID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, service_id, domain, status, verification_type, verification_token,
		       is_primary, ssl_status, COALESCE(ssl_error, ''), verified_at, created_at, updated_at
		FROM custom_domains
		WHERE service_id = $1
		ORDER BY is_primary DESC, created_at ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var domains []types.CustomDomain
	for rows.Next() {
		var d types.CustomDomain
		if err := rows.Scan(
			&d.ID, &d.ServiceID, &d.Domain, &d.Status, &d.VerificationType, &d.VerificationToken,
			&d.IsPrimary, &d.SSLStatus, &d.SSLError, &d.VerifiedAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		domains = append(domains, d)
	}

	return domains, total, nil
}

func (r *PostgresRepository) UpdateDomainStatus(ctx context.Context, id string, status types.DomainStatus, sslStatus types.SSLStatus, sslError string, verifiedAt *time.Time) error {
	query := `
		UPDATE custom_domains
		SET status = $1, ssl_status = $2, ssl_error = $3, verified_at = $4, updated_at = $5
		WHERE id = $6
	`
	res, err := r.db.ExecContext(ctx, query, status, sslStatus, sslError, verifiedAt, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrDomainNotFound
	}
	return nil
}

func (r *PostgresRepository) DeleteDomain(ctx context.Context, id string) error {
	query := `DELETE FROM custom_domains WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrDomainNotFound
	}
	return nil
}

func (r *PostgresRepository) SetPrimaryDomain(ctx context.Context, serviceID, domainID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Reset is_primary on all domains for this service
	_, err = tx.ExecContext(ctx, `UPDATE custom_domains SET is_primary = false, updated_at = $1 WHERE service_id = $2`, time.Now().UTC(), serviceID)
	if err != nil {
		return err
	}

	// 2. Set is_primary on target domain
	res, err := tx.ExecContext(ctx, `UPDATE custom_domains SET is_primary = true, updated_at = $1 WHERE id = $2 AND service_id = $3`, time.Now().UTC(), domainID, serviceID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrDomainNotFound
	}

	return tx.Commit()
}

func (r *PostgresRepository) UpsertIngressRoute(ctx context.Context, route *types.IngressRoute) error {
	if route.ID == "" {
		route.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if route.CreatedAt.IsZero() {
		route.CreatedAt = now
	}
	route.UpdatedAt = now
	route.LastSyncedAt = now
	route.HostPattern = strings.ToLower(strings.TrimSpace(route.HostPattern))

	query := `
		INSERT INTO ingress_routes (
			id, service_id, domain_id, host_pattern, upstream_url, is_active, last_synced_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (host_pattern) DO UPDATE SET
			service_id = EXCLUDED.service_id,
			domain_id = EXCLUDED.domain_id,
			upstream_url = EXCLUDED.upstream_url,
			is_active = EXCLUDED.is_active,
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(
		ctx, query,
		route.ID, route.ServiceID, route.DomainID, route.HostPattern, route.UpstreamURL,
		route.IsActive, route.LastSyncedAt, route.CreatedAt, route.UpdatedAt,
	).Scan(&route.ID, &route.CreatedAt, &route.UpdatedAt)
}

func (r *PostgresRepository) GetIngressRouteByHost(ctx context.Context, hostPattern string) (*types.IngressRoute, error) {
	query := `
		SELECT id, service_id, domain_id, host_pattern, upstream_url, is_active, last_synced_at, created_at, updated_at
		FROM ingress_routes
		WHERE host_pattern = $1
	`
	var route types.IngressRoute
	cleanHost := strings.ToLower(strings.TrimSpace(hostPattern))
	err := r.db.QueryRowContext(ctx, query, cleanHost).Scan(
		&route.ID, &route.ServiceID, &route.DomainID, &route.HostPattern, &route.UpstreamURL,
		&route.IsActive, &route.LastSyncedAt, &route.CreatedAt, &route.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRouteNotFound
		}
		return nil, err
	}
	return &route, nil
}

func (r *PostgresRepository) ListIngressRoutesByService(ctx context.Context, serviceID string) ([]types.IngressRoute, error) {
	query := `
		SELECT id, service_id, domain_id, host_pattern, upstream_url, is_active, last_synced_at, created_at, updated_at
		FROM ingress_routes
		WHERE service_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []types.IngressRoute
	for rows.Next() {
		var route types.IngressRoute
		if err := rows.Scan(
			&route.ID, &route.ServiceID, &route.DomainID, &route.HostPattern, &route.UpstreamURL,
			&route.IsActive, &route.LastSyncedAt, &route.CreatedAt, &route.UpdatedAt,
		); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (r *PostgresRepository) ListAllIngressRoutes(ctx context.Context, limit, offset int) ([]types.IngressRoute, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ingress_routes`).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, service_id, domain_id, host_pattern, upstream_url, is_active, last_synced_at, created_at, updated_at
		FROM ingress_routes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var routes []types.IngressRoute
	for rows.Next() {
		var route types.IngressRoute
		if err := rows.Scan(
			&route.ID, &route.ServiceID, &route.DomainID, &route.HostPattern, &route.UpstreamURL,
			&route.IsActive, &route.LastSyncedAt, &route.CreatedAt, &route.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		routes = append(routes, route)
	}
	return routes, total, nil
}

func (r *PostgresRepository) DeleteIngressRoute(ctx context.Context, hostPattern string) error {
	query := `DELETE FROM ingress_routes WHERE host_pattern = $1`
	cleanHost := strings.ToLower(strings.TrimSpace(hostPattern))
	res, err := r.db.ExecContext(ctx, query, cleanHost)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrRouteNotFound
	}
	return nil
}

func (r *PostgresRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	query := `
		SELECT id, project_id, name, slug, type, runtime_tier, repo_url, branch, port,
		       min_replicas, max_replicas, cpu_cores, memory_mb, disk_mb,
		       health_check_path, active_deployment_id, created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var (
		s                                types.Service
		projID, repoURL, branch, healthPath, activeDepID sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&s.ID, &projID, &s.Name, &s.Slug, &s.Type, &s.RuntimeTier, &repoURL, &branch, &s.Port,
		&s.MinReplicas, &s.MaxReplicas, &s.Resources.CPUCores, &s.Resources.MemoryMB, &s.Resources.DiskMB,
		&healthPath, &activeDepID, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}
	if projID.Valid {
		s.ProjectID = &projID.String
	}
	if repoURL.Valid {
		s.RepoURL = repoURL.String
	}
	if branch.Valid {
		s.Branch = branch.String
	}
	if healthPath.Valid {
		s.HealthCheck.Path = healthPath.String
	}
	if activeDepID.Valid {
		s.ActiveDeploymentID = &activeDepID.String
	}
	return &s, nil
}

func (r *PostgresRepository) GetActiveDeploymentForService(ctx context.Context, serviceID string) (*types.Deployment, error) {
	query := `
		SELECT d.id, d.service_id, d.build_id, d.status, COALESCE(d.nomad_job_id, ''), COALESCE(d.active_url, ''), d.replicas,
		       COALESCE(d.error_message, ''), d.created_at, d.updated_at
		FROM deployments d
		JOIN services s ON s.active_deployment_id = d.id
		WHERE s.id = $1
	`
	var d types.Deployment
	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(
		&d.ID, &d.ServiceID, &d.BuildID, &d.Status, &d.NomadJobID, &d.ActiveURL, &d.Replicas,
		&d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No active deployment yet
		}
		return nil, err
	}
	return &d, nil
}
