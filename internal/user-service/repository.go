package userservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrNotFound                = errors.New("resource not found")
	ErrAlreadyExists           = errors.New("resource already exists")
	ErrUnauthorized            = errors.New("unauthorized action")
	ErrAlreadyMember           = errors.New("user is already a member of this organization")
	ErrInvalidInvitation       = errors.New("invitation is invalid, expired, or revoked")
	ErrInvitationExpired       = errors.New("invitation has expired")
	ErrInvitationEmailMismatch = errors.New("this invitation was sent to a different email address")
	ErrPendingInvitationExists = errors.New("a pending invitation already exists for this email")
)

// Repository interface for user and organization operations
type Repository interface {
	GetUser(ctx context.Context, userID string) (*types.User, error)
	UpdateUser(ctx context.Context, user *types.User) error
	CreateOrganization(ctx context.Context, org *types.Organization) error
	GetOrganization(ctx context.Context, orgID string) (*types.Organization, error)
	ListUserOrganizations(ctx context.Context, userID string) ([]OrgDetail, error)
	AddOrgMember(ctx context.Context, orgID, userID, role string) error
	ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberInfo, error)
	GetMemberRole(ctx context.Context, orgID, userID string) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*types.User, error)

	// Invitation operations
	CreateInvitation(ctx context.Context, inv *types.OrganizationInvitation, tokenHash string) error
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*types.OrganizationInvitation, error)
	GetPendingInvitationByEmail(ctx context.Context, orgID, email string) (*types.OrganizationInvitation, error)
	ListOrgInvitations(ctx context.Context, orgID string) ([]types.OrganizationInvitation, error)
	UpdateInvitationStatus(ctx context.Context, invitationID string, status types.InvitationStatus) error
	GetInvitation(ctx context.Context, invitationID string) (*types.OrganizationInvitation, error)
}

// PostgresRepository implements Repository using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository initializes a Postgres repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) GetUser(ctx context.Context, userID string) (*types.User, error) {
	query := `SELECT id, email, name, COALESCE(avatar_url, ''), created_at, updated_at FROM users WHERE id = $1`
	var u types.User
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &u, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	query := `SELECT id, email, name, COALESCE(avatar_url, ''), created_at, updated_at FROM users WHERE email = $1`
	var u types.User
	if err := r.db.QueryRowContext(ctx, query, email).Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &u, nil
}

func (r *PostgresRepository) UpdateUser(ctx context.Context, user *types.User) error {
	query := `UPDATE users SET name = $1, avatar_url = $2, updated_at = $3 WHERE id = $4`
	_, err := r.db.ExecContext(ctx, query, user.Name, user.AvatarURL, user.UpdatedAt, user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateOrganization(ctx context.Context, org *types.Organization) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start tx: %w", err)
	}
	defer tx.Rollback()

	queryOrg := `INSERT INTO organizations (id, name, slug, owner_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := tx.ExecContext(ctx, queryOrg, org.ID, org.Name, org.Slug, org.OwnerID, org.CreatedAt, org.UpdatedAt); err != nil {
		return fmt.Errorf("failed to insert org: %w", err)
	}

	queryMember := `INSERT INTO organization_members (organization_id, user_id, role, created_at) VALUES ($1, $2, 'owner', $3)`
	if _, err := tx.ExecContext(ctx, queryMember, org.ID, org.OwnerID, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to insert org owner: %w", err)
	}

	return tx.Commit()
}

func (r *PostgresRepository) GetOrganization(ctx context.Context, orgID string) (*types.Organization, error) {
	query := `SELECT id, name, slug, owner_id, created_at, updated_at FROM organizations WHERE id = $1`
	var o types.Organization
	if err := r.db.QueryRowContext(ctx, query, orgID).Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get org: %w", err)
	}
	return &o, nil
}

func (r *PostgresRepository) ListUserOrganizations(ctx context.Context, userID string) ([]OrgDetail, error) {
	query := `
		SELECT o.id, o.name, o.slug, o.owner_id, o.created_at, o.updated_at, m.role,
		       (SELECT COUNT(*) FROM organization_members WHERE organization_id = o.id) AS member_count
		FROM organizations o
		JOIN organization_members m ON m.organization_id = o.id
		WHERE m.user_id = $1
		ORDER BY o.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user orgs: %w", err)
	}
	defer rows.Close()

	var orgs []OrgDetail
	for rows.Next() {
		var od OrgDetail
		if err := rows.Scan(&od.ID, &od.Name, &od.Slug, &od.OwnerID, &od.CreatedAt, &od.UpdatedAt, &od.Role, &od.MemberCount); err != nil {
			return nil, fmt.Errorf("failed to scan org row: %w", err)
		}
		orgs = append(orgs, od)
	}
	return orgs, nil
}

func (r *PostgresRepository) AddOrgMember(ctx context.Context, orgID, userID, role string) error {
	query := `INSERT INTO organization_members (organization_id, user_id, role, created_at) VALUES ($1, $2, $3, $4)`
	_, err := r.db.ExecContext(ctx, query, orgID, userID, role, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberInfo, error) {
	query := `
		SELECT m.id, u.id, u.email, u.name, m.role, m.created_at
		FROM organization_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = $1
		ORDER BY m.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	var members []OrgMemberInfo
	for rows.Next() {
		var m OrgMemberInfo
		if err := rows.Scan(&m.ID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

func (r *PostgresRepository) GetMemberRole(ctx context.Context, orgID, userID string) (string, error) {
	query := `SELECT role FROM organization_members WHERE organization_id = $1 AND user_id = $2`
	var role string
	if err := r.db.QueryRowContext(ctx, query, orgID, userID).Scan(&role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get member role: %w", err)
	}
	return role, nil
}

func (r *PostgresRepository) CreateInvitation(ctx context.Context, inv *types.OrganizationInvitation, tokenHash string) error {
	query := `INSERT INTO organization_invitations (id, organization_id, email, role, invited_by, token_hash, status, expires_at, created_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, query, inv.ID, inv.OrganizationID, inv.Email, inv.Role, inv.InvitedBy, tokenHash, string(inv.Status), inv.ExpiresAt, inv.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*types.OrganizationInvitation, error) {
	query := `SELECT id, organization_id, email, role, invited_by, status, expires_at, created_at
	          FROM organization_invitations WHERE token_hash = $1`
	var inv types.OrganizationInvitation
	var status string
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.InvitedBy, &status, &inv.ExpiresAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get invitation by token: %w", err)
	}
	inv.Status = types.InvitationStatus(status)
	return &inv, nil
}

func (r *PostgresRepository) GetPendingInvitationByEmail(ctx context.Context, orgID, email string) (*types.OrganizationInvitation, error) {
	query := `SELECT id, organization_id, email, role, invited_by, status, expires_at, created_at
	          FROM organization_invitations 
	          WHERE organization_id = $1 AND email = $2 AND status = 'pending' AND expires_at > CURRENT_TIMESTAMP`
	var inv types.OrganizationInvitation
	var status string
	err := r.db.QueryRowContext(ctx, query, orgID, email).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.InvitedBy, &status, &inv.ExpiresAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to check pending invitation: %w", err)
	}
	inv.Status = types.InvitationStatus(status)
	return &inv, nil
}

func (r *PostgresRepository) ListOrgInvitations(ctx context.Context, orgID string) ([]types.OrganizationInvitation, error) {
	query := `SELECT id, organization_id, email, role, invited_by, status, expires_at, created_at
	          FROM organization_invitations 
	          WHERE organization_id = $1
	          ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	defer rows.Close()

	var list []types.OrganizationInvitation
	for rows.Next() {
		var inv types.OrganizationInvitation
		var status string
		if err := rows.Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.InvitedBy, &status, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan invitation: %w", err)
		}
		inv.Status = types.InvitationStatus(status)
		list = append(list, inv)
	}
	return list, nil
}

func (r *PostgresRepository) UpdateInvitationStatus(ctx context.Context, invitationID string, status types.InvitationStatus) error {
	query := `UPDATE organization_invitations SET status = $1 WHERE id = $2`
	res, err := r.db.ExecContext(ctx, query, string(status), invitationID)
	if err != nil {
		return fmt.Errorf("failed to update invitation status: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetInvitation(ctx context.Context, invitationID string) (*types.OrganizationInvitation, error) {
	query := `SELECT id, organization_id, email, role, invited_by, status, expires_at, created_at
	          FROM organization_invitations WHERE id = $1`
	var inv types.OrganizationInvitation
	var status string
	err := r.db.QueryRowContext(ctx, query, invitationID).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.InvitedBy, &status, &inv.ExpiresAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get invitation: %w", err)
	}
	inv.Status = types.InvitationStatus(status)
	return &inv, nil
}

// MemoryRepository implements Repository in-memory for testing
type MemoryRepository struct {
	mu           sync.RWMutex
	users        map[string]*types.User
	orgs         map[string]*types.Organization
	memberships  map[string]map[string]string // orgID -> userID -> role
	invites      map[string]*types.OrganizationInvitation // inviteID -> invite
	inviteHashes map[string]string                        // tokenHash -> inviteID
}

// NewMemoryRepository initializes an in-memory repository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		users:        make(map[string]*types.User),
		orgs:         make(map[string]*types.Organization),
		memberships:  make(map[string]map[string]string),
		invites:      make(map[string]*types.OrganizationInvitation),
		inviteHashes: make(map[string]string),
	}
}

func (m *MemoryRepository) SeedUser(u *types.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
}

func (m *MemoryRepository) GetUser(ctx context.Context, userID string) (*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[userID]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (m *MemoryRepository) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemoryRepository) UpdateUser(ctx context.Context, user *types.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *MemoryRepository) CreateOrganization(ctx context.Context, org *types.Organization) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs[org.ID] = org
	if m.memberships[org.ID] == nil {
		m.memberships[org.ID] = make(map[string]string)
	}
	m.memberships[org.ID][org.OwnerID] = "owner"
	return nil
}

func (m *MemoryRepository) GetOrganization(ctx context.Context, orgID string) (*types.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orgs[orgID]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func (m *MemoryRepository) ListUserOrganizations(ctx context.Context, userID string) ([]OrgDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []OrgDetail
	for orgID, members := range m.memberships {
		if role, ok := members[userID]; ok {
			org := m.orgs[orgID]
			result = append(result, OrgDetail{
				Organization: *org,
				Role:         role,
				MemberCount:  len(members),
			})
		}
	}
	return result, nil
}

func (m *MemoryRepository) AddOrgMember(ctx context.Context, orgID, userID, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.memberships[orgID] == nil {
		m.memberships[orgID] = make(map[string]string)
	}
	m.memberships[orgID][userID] = role
	return nil
}

func (m *MemoryRepository) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members, ok := m.memberships[orgID]
	if !ok {
		return nil, nil
	}

	var result []OrgMemberInfo
	for userID, role := range members {
		u := m.users[userID]
		email, name := "", ""
		if u != nil {
			email = u.Email
			name = u.Name
		}
		result = append(result, OrgMemberInfo{
			ID:       uuid.New().String(),
			UserID:   userID,
			Email:    email,
			Name:     name,
			Role:     role,
			JoinedAt: time.Now().UTC(),
		})
	}
	return result, nil
}

func (m *MemoryRepository) GetMemberRole(ctx context.Context, orgID, userID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	members, ok := m.memberships[orgID]
	if !ok {
		return "", ErrNotFound
	}
	role, ok := members[userID]
	if !ok {
		return "", ErrNotFound
	}
	return role, nil
}

func (m *MemoryRepository) CreateInvitation(ctx context.Context, inv *types.OrganizationInvitation, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *inv
	m.invites[inv.ID] = &copy
	m.inviteHashes[tokenHash] = inv.ID
	return nil
}

func (m *MemoryRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*types.OrganizationInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.inviteHashes[tokenHash]
	if !ok {
		return nil, ErrNotFound
	}
	inv, ok := m.invites[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *inv
	return &copy, nil
}

func (m *MemoryRepository) GetPendingInvitationByEmail(ctx context.Context, orgID, email string) (*types.OrganizationInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now().UTC()
	for _, inv := range m.invites {
		if inv.OrganizationID == orgID && strings.EqualFold(inv.Email, email) && inv.Status == types.InvitationStatusPending && now.Before(inv.ExpiresAt) {
			copy := *inv
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemoryRepository) ListOrgInvitations(ctx context.Context, orgID string) ([]types.OrganizationInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []types.OrganizationInvitation
	for _, inv := range m.invites {
		if inv.OrganizationID == orgID {
			list = append(list, *inv)
		}
	}
	return list, nil
}

func (m *MemoryRepository) UpdateInvitationStatus(ctx context.Context, invitationID string, status types.InvitationStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[invitationID]
	if !ok {
		return ErrNotFound
	}
	inv.Status = status
	return nil
}

func (m *MemoryRepository) GetInvitation(ctx context.Context, invitationID string) (*types.OrganizationInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inv, ok := m.invites[invitationID]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *inv
	return &copy, nil
}

