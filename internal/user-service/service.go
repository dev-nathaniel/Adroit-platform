package userservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrOrgNameRequired  = errors.New("organization name is required")
	ErrInvalidRole      = errors.New("role must be 'admin' or 'member'")
	ErrPermissionDenied = errors.New("only owners and admins can invite members")
	ErrInvalidEmail     = errors.New("invalid email address")
)

// Option defines a functional option for Service configuration
type Option func(*Service)

// WithPublisher attaches an event publisher for domain event emission
func WithPublisher(pub events.Publisher) Option {
	return func(s *Service) {
		if pub != nil {
			s.publisher = pub
		}
	}
}

// WithStorageProvider configures media/avatar storage provider
func WithStorageProvider(storage StorageProvider) Option {
	return func(s *Service) {
		if storage != nil {
			s.storage = storage
		}
	}
}

// Service defines user and organization business logic
type Service struct {
	repo      Repository
	publisher events.Publisher
	storage   StorageProvider
}

// NewService creates a new user service instance with optional functional configurations
func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{
		repo:      repo,
		publisher: events.NewNoopPublisher(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.storage == nil {
		localStore, err := NewLocalStorage("data/uploads/avatars", "/api/v1/users/avatars")
		if err != nil {
			s.storage = NewInMemStorage("/api/v1/users/avatars")
		} else {
			s.storage = localStore
		}
	}
	return s
}

// Storage returns the active StorageProvider
func (s *Service) Storage() StorageProvider {
	return s.storage
}

// GetProfile retrieves user profile
func (s *Service) GetProfile(ctx context.Context, userID string) (*types.User, error) {
	return s.repo.GetUser(ctx, userID)
}

// UpdateProfile updates user profile
func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (*types.User, error) {
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Name) != "" {
		user.Name = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.AvatarURL) != "" {
		user.AvatarURL = strings.TrimSpace(req.AvatarURL)
	}
	user.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// UploadAvatar persists an avatar file to storage and updates user profile
func (s *Service) UploadAvatar(ctx context.Context, userID, filename string, data io.Reader) (*types.User, string, error) {
	avatarURL, err := s.storage.Save(ctx, filename, data)
	if err != nil {
		return nil, "", fmt.Errorf("failed to save avatar to storage: %w", err)
	}

	user, err := s.UpdateAvatar(ctx, userID, avatarURL)
	if err != nil {
		_ = s.storage.Delete(ctx, filename)
		return nil, "", err
	}
	return user, avatarURL, nil
}

// UpdateAvatar updates a user's avatar URL
func (s *Service) UpdateAvatar(ctx context.Context, userID, avatarURL string) (*types.User, error) {
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	user.AvatarURL = strings.TrimSpace(avatarURL)
	user.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// RemoveAvatar clears a user's avatar URL
func (s *Service) RemoveAvatar(ctx context.Context, userID string) (*types.User, error) {
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	user.AvatarURL = ""
	user.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// CreateOrganization creates a new team/workspace
func (s *Service) CreateOrganization(ctx context.Context, ownerID string, req CreateOrgRequest) (*types.Organization, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrOrgNameRequired
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name) + "-" + uuid.New().String()[:6]
	}

	now := time.Now().UTC()
	org := &types.Organization{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      slug,
		OwnerID:   ownerID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateOrganization(ctx, org); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return org, nil
}

// GetOrganization returns organization if caller is a member
func (s *Service) GetOrganization(ctx context.Context, userID, orgID string) (*types.Organization, string, error) {
	role, err := s.repo.GetMemberRole(ctx, orgID, userID)
	if err != nil {
		return nil, "", ErrUnauthorized
	}

	org, err := s.repo.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, "", err
	}

	return org, role, nil
}

// ListOrganizations returns all organizations the user belongs to
func (s *Service) ListOrganizations(ctx context.Context, userID string) ([]OrgDetail, error) {
	return s.repo.ListUserOrganizations(ctx, userID)
}

// AddMember invites or adds a user to an organization
func (s *Service) AddMember(ctx context.Context, callerID, orgID string, req AddMemberRequest) error {
	callerRole, err := s.repo.GetMemberRole(ctx, orgID, callerID)
	if err != nil || (callerRole != "owner" && callerRole != "admin") {
		return ErrPermissionDenied
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "admin" && role != "member" {
		return ErrInvalidRole
	}

	targetUser, err := s.repo.GetUserByEmail(ctx, strings.TrimSpace(strings.ToLower(req.Email)))
	if err != nil {
		return fmt.Errorf("user with email %s not found: %w", req.Email, err)
	}

	// Check if already member
	if _, err := s.repo.GetMemberRole(ctx, orgID, targetUser.ID); err == nil {
		return ErrAlreadyMember
	}

	return s.repo.AddOrgMember(ctx, orgID, targetUser.ID, role)
}

// ListMembers returns all members of an organization
func (s *Service) ListMembers(ctx context.Context, callerID, orgID string) ([]OrgMemberInfo, error) {
	// Verify caller belongs to org
	if _, err := s.repo.GetMemberRole(ctx, orgID, callerID); err != nil {
		return nil, ErrUnauthorized
	}
	return s.repo.ListOrgMembers(ctx, orgID)
}

// CreateInvitation creates a secure pending invitation to an organization
func (s *Service) CreateInvitation(ctx context.Context, callerID, orgID string, req CreateInvitationRequest) (*InvitationResponse, error) {
	callerRole, err := s.repo.GetMemberRole(ctx, orgID, callerID)
	if err != nil || (callerRole != "owner" && callerRole != "admin") {
		return nil, ErrPermissionDenied
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "admin" && role != "member" {
		return nil, ErrInvalidRole
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidEmail
	}

	// Verify organization exists
	if _, err := s.repo.GetOrganization(ctx, orgID); err != nil {
		return nil, err
	}

	// Check if user already exists and is already a member
	if existingUser, err := s.repo.GetUserByEmail(ctx, email); err == nil && existingUser != nil {
		if _, err := s.repo.GetMemberRole(ctx, orgID, existingUser.ID); err == nil {
			return nil, ErrAlreadyMember
		}
	}

	// Check if active pending invitation already exists for this email in this org
	if _, err := s.repo.GetPendingInvitationByEmail(ctx, orgID, email); err == nil {
		return nil, ErrPendingInvitationExists
	}

	rawToken, tokenHash, err := generateInviteToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	invitation := &types.OrganizationInvitation{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		Email:          email,
		Role:           role,
		InvitedBy:      callerID,
		Status:         types.InvitationStatusPending,
		ExpiresAt:      now.Add(7 * 24 * time.Hour), // 7-day expiration window
		CreatedAt:      now,
	}

	if err := s.repo.CreateInvitation(ctx, invitation, tokenHash); err != nil {
		return nil, fmt.Errorf("failed to save invitation: %w", err)
	}

	inviteURL := "/invite?token=" + rawToken

	// Fetch org and inviter details for notification payload
	orgName := ""
	if org, err := s.repo.GetOrganization(ctx, orgID); err == nil && org != nil {
		orgName = org.Name
	}
	inviterName, inviterEmail := "", ""
	if inviter, err := s.repo.GetUser(ctx, callerID); err == nil && inviter != nil {
		inviterName = inviter.Name
		inviterEmail = inviter.Email
	}

	// Asynchronously emit domain event to NATS JetStream for Notification Service
	if s.publisher != nil {
		eventPayload := events.OrgMemberInvitedPayload{
			InvitationID:     invitation.ID,
			OrganizationID:   orgID,
			OrganizationName: orgName,
			Email:            email,
			Role:             string(role),
			InvitedByID:      callerID,
			InvitedByName:    inviterName,
			InvitedByEmail:   inviterEmail,
			InviteURL:        inviteURL,
			ExpiresAt:        invitation.ExpiresAt,
		}
		_ = s.publisher.Publish(ctx, events.SubjectOrgMemberInvited, eventPayload)
	}

	return &InvitationResponse{
		OrganizationInvitation: *invitation,
		Token:                  rawToken,
		InviteURL:              inviteURL,
	}, nil
}

// GetInvitationPreview retrieves public information about an invitation using the raw token
func (s *Service) GetInvitationPreview(ctx context.Context, rawToken string) (*InvitationPreview, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrInvalidInvitation
	}

	tokenHash := hashInviteToken(rawToken)
	inv, err := s.repo.GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidInvitation
	}

	if inv.Status == types.InvitationStatusExpired || time.Now().UTC().After(inv.ExpiresAt) {
		_ = s.repo.UpdateInvitationStatus(ctx, inv.ID, types.InvitationStatusExpired)
		return nil, ErrInvitationExpired
	}

	if inv.Status == types.InvitationStatusRevoked {
		return nil, ErrInvalidInvitation
	}

	org, err := s.repo.GetOrganization(ctx, inv.OrganizationID)
	if err != nil {
		return nil, err
	}

	invitedByName := "Workspace Admin"
	if inviter, err := s.repo.GetUser(ctx, inv.InvitedBy); err == nil && inviter != nil {
		invitedByName = inviter.Name
	}

	userExists := false
	if existingUser, err := s.repo.GetUserByEmail(ctx, inv.Email); err == nil && existingUser != nil {
		userExists = true
	}

	return &InvitationPreview{
		ID:               inv.ID,
		OrganizationID:   org.ID,
		OrganizationName: org.Name,
		Email:            inv.Email,
		Role:             inv.Role,
		InvitedByName:    invitedByName,
		ExpiresAt:        inv.ExpiresAt,
		Token:            rawToken,
		Status:           string(inv.Status),
		UserExists:       userExists,
	}, nil
}

// AcceptInvitation allows an authenticated user whose email matches the invite to join the org
func (s *Service) AcceptInvitation(ctx context.Context, userID, rawToken string) (*AcceptInvitationResponse, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrUnauthorized
	}

	user, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, ErrNotFound
	}

	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrInvalidInvitation
	}

	tokenHash := hashInviteToken(rawToken)
	inv, err := s.repo.GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidInvitation
	}

	if inv.Status != types.InvitationStatusPending {
		return nil, ErrInvalidInvitation
	}

	if time.Now().UTC().After(inv.ExpiresAt) {
		_ = s.repo.UpdateInvitationStatus(ctx, inv.ID, types.InvitationStatusExpired)
		return nil, ErrInvitationExpired
	}

	// Security: Caller's registered email must match the invitation's destination email
	if !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(inv.Email)) {
		return nil, ErrInvitationEmailMismatch
	}

	// Add member to org
	if err := s.repo.AddOrgMember(ctx, inv.OrganizationID, user.ID, inv.Role); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already") && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, fmt.Errorf("failed to join organization: %w", err)
		}
	}

	// Mark invitation accepted
	if err := s.repo.UpdateInvitationStatus(ctx, inv.ID, types.InvitationStatusAccepted); err != nil {
		return nil, fmt.Errorf("failed to update invitation status: %w", err)
	}

	org, err := s.repo.GetOrganization(ctx, inv.OrganizationID)
	orgName := ""
	if err == nil && org != nil {
		orgName = org.Name
	}

	return &AcceptInvitationResponse{
		OrganizationID:   inv.OrganizationID,
		OrganizationName: orgName,
		Role:             inv.Role,
		Message:          fmt.Sprintf("Successfully joined %s as %s", orgName, inv.Role),
	}, nil
}

// ListInvitations lists pending invitations for an organization (admin/owner only)
func (s *Service) ListInvitations(ctx context.Context, callerID, orgID string) ([]types.OrganizationInvitation, error) {
	callerRole, err := s.repo.GetMemberRole(ctx, orgID, callerID)
	if err != nil || (callerRole != "owner" && callerRole != "admin") {
		return nil, ErrPermissionDenied
	}
	return s.repo.ListOrgInvitations(ctx, orgID)
}

// RevokeInvitation revokes a pending invitation (admin/owner only)
func (s *Service) RevokeInvitation(ctx context.Context, callerID, orgID, invitationID string) error {
	callerRole, err := s.repo.GetMemberRole(ctx, orgID, callerID)
	if err != nil || (callerRole != "owner" && callerRole != "admin") {
		return ErrPermissionDenied
	}

	inv, err := s.repo.GetInvitation(ctx, invitationID)
	if err != nil {
		return err
	}
	if inv.OrganizationID != orgID {
		return ErrNotFound
	}

	return s.repo.UpdateInvitationStatus(ctx, invitationID, types.InvitationStatusRevoked)
}

func generateInviteToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate random token: %w", err)
	}
	token := hex.EncodeToString(b)
	hash := hashInviteToken(token)
	return token, hash, nil
}

func hashInviteToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}


var nonAlphaNumRegex = regexp.MustCompile(`[^a-z0-9]+`)

func generateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphaNumRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	return slug
}
