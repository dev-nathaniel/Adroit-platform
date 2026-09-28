package authservice

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrPasswordTooWeak = errors.New("password must be at least 8 characters long")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// Service defines auth business logic operations
type Service struct {
	repo          Repository
	jwtSecret     string
	jwtExpiration time.Duration
}

// NewService creates a new auth service instance
func NewService(repo Repository, jwtSecret string, jwtExpiration time.Duration) *Service {
	return &Service{
		repo:          repo,
		jwtSecret:     jwtSecret,
		jwtExpiration: jwtExpiration,
	}
}

// Register creates a new user, hashes password, provisions a default workspace, and returns JWT
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if _, err := mail.ParseAddress(req.Email); err != nil {
		return nil, ErrInvalidEmail
	}

	if len(req.Password) < 8 {
		return nil, ErrPasswordTooWeak
	}

	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}

	// Check if user already exists
	if _, err := s.repo.GetUserByEmail(ctx, req.Email); err == nil {
		return nil, ErrUserAlreadyExists
	}

	// Hash password
	passwordHash, err := identity.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	userID := uuid.New().String()

	account := &UserAccount{
		ID:           userID,
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateUser(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create default organization
	slug := generateSlug(req.Name) + "-" + userID[:6]
	org := &types.Organization{
		ID:        uuid.New().String(),
		Name:      req.Name + "'s Workspace",
		Slug:      slug,
		OwnerID:   userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = s.repo.CreateOrganization(ctx, org)

	// Issue JWT token
	token, err := identity.GenerateJWT(userID, req.Email, s.jwtSecret, s.jwtExpiration)
	if err != nil {
		return nil, fmt.Errorf("failed to issue token: %w", err)
	}

	return &AuthResponse{
		Token: token,
		User: types.User{
			ID:        account.ID,
			Email:     account.Email,
			Name:      account.Name,
			CreatedAt: account.CreatedAt,
			UpdatedAt: account.UpdatedAt,
		},
		Organization: org,
	}, nil
}

// Login authenticates credentials and issues a JWT token
func (s *Service) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	account, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !identity.CheckPassword(req.Password, account.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	token, err := identity.GenerateJWT(account.ID, account.Email, s.jwtSecret, s.jwtExpiration)
	if err != nil {
		return nil, fmt.Errorf("failed to issue token: %w", err)
	}

	return &AuthResponse{
		Token: token,
		User: types.User{
			ID:        account.ID,
			Email:     account.Email,
			Name:      account.Name,
			AvatarURL: account.AvatarURL,
			CreatedAt: account.CreatedAt,
			UpdatedAt: account.UpdatedAt,
		},
	}, nil
}

// GetMe retrieves the profile of the authenticated user
func (s *Service) GetMe(ctx context.Context, userID string) (*types.User, error) {
	account, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &types.User{
		ID:        account.ID,
		Email:     account.Email,
		Name:      account.Name,
		AvatarURL: account.AvatarURL,
		CreatedAt: account.CreatedAt,
		UpdatedAt: account.UpdatedAt,
	}, nil
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
