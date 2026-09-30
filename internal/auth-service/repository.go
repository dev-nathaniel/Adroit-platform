package authservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
)

// Repository interface for auth storage
type Repository interface {
	CreateUser(ctx context.Context, account *UserAccount) error
	GetUserByEmail(ctx context.Context, email string) (*UserAccount, error)
	GetUserByID(ctx context.Context, id string) (*UserAccount, error)
	GetUserByGitHubID(ctx context.Context, githubID string) (*UserAccount, error)
	LinkGitHubAccount(ctx context.Context, userID, githubID, githubUsername, avatarURL, accessToken string) error
	UpdateGitHubToken(ctx context.Context, userID, accessToken string) error
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	CreateOrganization(ctx context.Context, org *types.Organization) error
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a PostgreSQL repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, acc *UserAccount) error {
	query := `
		INSERT INTO users (id, email, name, password_hash, avatar_url, github_id, github_username, github_access_token, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	var passHash any
	if acc.PasswordHash != "" {
		passHash = acc.PasswordHash
	}
	var avatarURL any
	if acc.AvatarURL != "" {
		avatarURL = acc.AvatarURL
	}
	var githubID any
	if acc.GitHubID != "" {
		githubID = acc.GitHubID
	}
	var githubUsername any
	if acc.GitHubUsername != "" {
		githubUsername = acc.GitHubUsername
	}
	var githubToken any
	if acc.GitHubAccessToken != "" {
		githubToken = acc.GitHubAccessToken
	}

	_, err := r.db.ExecContext(ctx, query,
		acc.ID, acc.Email, acc.Name, passHash, avatarURL, githubID, githubUsername, githubToken, acc.CreatedAt, acc.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*UserAccount, error) {
	query := `
		SELECT id, email, name, COALESCE(password_hash, ''), COALESCE(avatar_url, ''), COALESCE(github_id, ''), COALESCE(github_username, ''), COALESCE(github_access_token, ''), created_at, updated_at
		FROM users
		WHERE email = $1
	`
	row := r.db.QueryRowContext(ctx, query, email)

	var acc UserAccount
	if err := row.Scan(&acc.ID, &acc.Email, &acc.Name, &acc.PasswordHash, &acc.AvatarURL, &acc.GitHubID, &acc.GitHubUsername, &acc.GitHubAccessToken, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}
	return &acc, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id string) (*UserAccount, error) {
	query := `
		SELECT id, email, name, COALESCE(password_hash, ''), COALESCE(avatar_url, ''), COALESCE(github_id, ''), COALESCE(github_username, ''), COALESCE(github_access_token, ''), created_at, updated_at
		FROM users
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var acc UserAccount
	if err := row.Scan(&acc.ID, &acc.Email, &acc.Name, &acc.PasswordHash, &acc.AvatarURL, &acc.GitHubID, &acc.GitHubUsername, &acc.GitHubAccessToken, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by id: %w", err)
	}
	return &acc, nil
}

func (r *PostgresRepository) GetUserByGitHubID(ctx context.Context, githubID string) (*UserAccount, error) {
	query := `
		SELECT id, email, name, COALESCE(password_hash, ''), COALESCE(avatar_url, ''), COALESCE(github_id, ''), COALESCE(github_username, ''), COALESCE(github_access_token, ''), created_at, updated_at
		FROM users
		WHERE github_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, githubID)

	var acc UserAccount
	if err := row.Scan(&acc.ID, &acc.Email, &acc.Name, &acc.PasswordHash, &acc.AvatarURL, &acc.GitHubID, &acc.GitHubUsername, &acc.GitHubAccessToken, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by github_id: %w", err)
	}
	return &acc, nil
}

func (r *PostgresRepository) LinkGitHubAccount(ctx context.Context, userID, githubID, githubUsername, avatarURL, accessToken string) error {
	query := `
		UPDATE users
		SET github_id = $2,
		    github_username = $3,
		    github_access_token = $4,
		    avatar_url = CASE WHEN avatar_url IS NULL OR avatar_url = '' THEN $5 ELSE avatar_url END,
		    updated_at = $6
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, userID, githubID, githubUsername, accessToken, avatarURL, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to link github account: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateGitHubToken(ctx context.Context, userID, accessToken string) error {
	query := `
		UPDATE users
		SET github_access_token = $2,
		    updated_at = $3
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, userID, accessToken, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update github token: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $2,
		    updated_at = $3
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, userID, passwordHash, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateOrganization(ctx context.Context, org *types.Organization) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	orgQuery := `
		INSERT INTO organizations (id, name, slug, owner_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	if _, err := tx.ExecContext(ctx, orgQuery, org.ID, org.Name, org.Slug, org.OwnerID, org.CreatedAt, org.UpdatedAt); err != nil {
		return fmt.Errorf("failed to insert organization: %w", err)
	}

	memberQuery := `
		INSERT INTO organization_members (organization_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`
	if _, err := tx.ExecContext(ctx, memberQuery, org.ID, org.OwnerID, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to insert owner membership: %w", err)
	}

	return tx.Commit()
}

// MemoryRepository provides thread-safe in-memory storage for testing and dev
type MemoryRepository struct {
	mu              sync.RWMutex
	usersByID       map[string]*UserAccount
	usersByEmail    map[string]*UserAccount
	usersByGitHubID map[string]*UserAccount
	organizations   map[string]*types.Organization
}

// NewMemoryRepository initializes an in-memory repository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:       make(map[string]*UserAccount),
		usersByEmail:    make(map[string]*UserAccount),
		usersByGitHubID: make(map[string]*UserAccount),
		organizations:   make(map[string]*types.Organization),
	}
}

func (m *MemoryRepository) CreateUser(ctx context.Context, acc *UserAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.usersByEmail[acc.Email]; exists {
		return ErrUserAlreadyExists
	}
	if acc.GitHubID != "" {
		if _, exists := m.usersByGitHubID[acc.GitHubID]; exists {
			return ErrUserAlreadyExists
		}
	}
	m.usersByID[acc.ID] = acc
	m.usersByEmail[acc.Email] = acc
	if acc.GitHubID != "" {
		m.usersByGitHubID[acc.GitHubID] = acc
	}
	return nil
}

func (m *MemoryRepository) GetUserByEmail(ctx context.Context, email string) (*UserAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc, exists := m.usersByEmail[email]
	if !exists {
		return nil, ErrUserNotFound
	}
	return acc, nil
}

func (m *MemoryRepository) GetUserByID(ctx context.Context, id string) (*UserAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc, exists := m.usersByID[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	return acc, nil
}

func (m *MemoryRepository) GetUserByGitHubID(ctx context.Context, githubID string) (*UserAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc, exists := m.usersByGitHubID[githubID]
	if !exists {
		return nil, ErrUserNotFound
	}
	return acc, nil
}

func (m *MemoryRepository) LinkGitHubAccount(ctx context.Context, userID, githubID, githubUsername, avatarURL, accessToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc, exists := m.usersByID[userID]
	if !exists {
		return ErrUserNotFound
	}
	acc.GitHubID = githubID
	acc.GitHubUsername = githubUsername
	acc.GitHubAccessToken = accessToken
	if acc.AvatarURL == "" {
		acc.AvatarURL = avatarURL
	}
	acc.UpdatedAt = time.Now().UTC()
	m.usersByGitHubID[githubID] = acc
	return nil
}

func (m *MemoryRepository) UpdateGitHubToken(ctx context.Context, userID, accessToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc, exists := m.usersByID[userID]
	if !exists {
		return ErrUserNotFound
	}
	acc.GitHubAccessToken = accessToken
	acc.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc, exists := m.usersByID[userID]
	if !exists {
		return ErrUserNotFound
	}
	acc.PasswordHash = passwordHash
	acc.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryRepository) CreateOrganization(ctx context.Context, org *types.Organization) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.organizations[org.ID] = org
	return nil
}
