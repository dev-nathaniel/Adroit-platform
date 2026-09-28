package userservice

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrOrgNameRequired = errors.New("organization name is required")
	ErrInvalidRole     = errors.New("role must be 'admin' or 'member'")
	ErrPermissionDenied = errors.New("only owners and admins can invite members")
)

// Service defines user and organization business logic
type Service struct {
	repo Repository
}

// NewService creates a new user service instance
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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
