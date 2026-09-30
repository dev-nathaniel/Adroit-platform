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
	ErrInvalidEmail        = errors.New("invalid email address")
	ErrPasswordTooWeak     = errors.New("password must be at least 8 characters long")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrGitHubNotConfigured = errors.New("github oauth is not configured")
	ErrInvalidOAuthCode    = errors.New("invalid oauth code")
)

// Option allows configuring optional Service components
type Option func(*Service)

// WithGitHubClient configures the GitHub OAuth client
func WithGitHubClient(client GitHubClient) Option {
	return func(s *Service) {
		s.githubClient = client
	}
}

// WithFrontendURL configures the redirect destination after OAuth callback
func WithFrontendURL(url string) Option {
	return func(s *Service) {
		s.frontendURL = url
	}
}

// Service defines auth business logic operations
type Service struct {
	repo          Repository
	jwtSecret     string
	jwtExpiration time.Duration
	githubClient  GitHubClient
	frontendURL   string
}

// NewService creates a new auth service instance with optional configurations
func NewService(repo Repository, jwtSecret string, jwtExpiration time.Duration, opts ...Option) *Service {
	s := &Service{
		repo:          repo,
		jwtSecret:     jwtSecret,
		jwtExpiration: jwtExpiration,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

	if account.PasswordHash == "" || !identity.CheckPassword(req.Password, account.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	token, err := identity.GenerateJWT(account.ID, account.Email, s.jwtSecret, s.jwtExpiration)
	if err != nil {
		return nil, fmt.Errorf("failed to issue token: %w", err)
	}

	return &AuthResponse{
		Token: token,
		User: types.User{
			ID:             account.ID,
			Email:          account.Email,
			Name:           account.Name,
			AvatarURL:      account.AvatarURL,
			GitHubUsername: account.GitHubUsername,
			CreatedAt:      account.CreatedAt,
			UpdatedAt:      account.UpdatedAt,
		},
	}, nil
}

// GetGitHubAuthURL returns the OAuth authorization URL
func (s *Service) GetGitHubAuthURL(state string) (*GitHubAuthURLResponse, error) {
	if s.githubClient == nil {
		return nil, ErrGitHubNotConfigured
	}
	authURL := s.githubClient.GetAuthURL(state)
	return &GitHubAuthURLResponse{
		URL:   authURL,
		State: state,
	}, nil
}

// GetFrontendURL returns the configured frontend redirect destination
func (s *Service) GetFrontendURL() string {
	return s.frontendURL
}

// AuthenticateWithGitHub exchanges a code from GitHub, retrieves user information,
// links or creates the account, provisions a workspace if new, and issues a JWT.
func (s *Service) AuthenticateWithGitHub(ctx context.Context, code string) (*AuthResponse, error) {
	if s.githubClient == nil {
		return nil, ErrGitHubNotConfigured
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrInvalidOAuthCode
	}

	// 1. Exchange temporary code for access token
	accessToken, err := s.githubClient.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange github code: %w", err)
	}

	// 2. Fetch user profile and primary email
	ghUser, err := s.githubClient.GetUser(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch github profile: %w", err)
	}

	ghIDStr := fmt.Sprintf("%d", ghUser.ID)
	email := strings.TrimSpace(strings.ToLower(ghUser.Email))
	name := strings.TrimSpace(ghUser.Name)
	if name == "" {
		name = ghUser.Login
	}

	now := time.Now().UTC()
	var account *UserAccount
	var defaultOrg *types.Organization

	// 3. Check if user already exists by GitHub ID
	existingUserByGH, err := s.repo.GetUserByGitHubID(ctx, ghIDStr)
	if err == nil && existingUserByGH != nil {
		account = existingUserByGH
		_ = s.repo.UpdateGitHubToken(ctx, account.ID, accessToken)
	} else {
		// 4. Check if user already exists by verified email (account linking)
		existingUserByEmail, err := s.repo.GetUserByEmail(ctx, email)
		if err == nil && existingUserByEmail != nil {
			account = existingUserByEmail
			_ = s.repo.LinkGitHubAccount(ctx, account.ID, ghIDStr, ghUser.Login, ghUser.AvatarURL, accessToken)
			account.GitHubID = ghIDStr
			account.GitHubUsername = ghUser.Login
			account.GitHubAccessToken = accessToken
			if account.AvatarURL == "" {
				account.AvatarURL = ghUser.AvatarURL
			}
		} else {
			// 5. Brand new user registration via GitHub
			userID := uuid.New().String()
			account = &UserAccount{
				ID:                userID,
				Email:             email,
				Name:              name,
				AvatarURL:         ghUser.AvatarURL,
				GitHubID:          ghIDStr,
				GitHubUsername:    ghUser.Login,
				GitHubAccessToken: accessToken,
				CreatedAt:         now,
				UpdatedAt:         now,
			}

			if err := s.repo.CreateUser(ctx, account); err != nil {
				return nil, fmt.Errorf("failed to create oauth user: %w", err)
			}

			// Automatically provision default workspace
			slug := generateSlug(account.Name) + "-" + userID[:6]
			defaultOrg = &types.Organization{
				ID:        uuid.New().String(),
				Name:      account.Name + "'s Workspace",
				Slug:      slug,
				OwnerID:   userID,
				CreatedAt: now,
				UpdatedAt: now,
			}
			_ = s.repo.CreateOrganization(ctx, defaultOrg)
		}
	}

	// 6. Issue JWT Token
	token, err := identity.GenerateJWT(account.ID, account.Email, s.jwtSecret, s.jwtExpiration)
	if err != nil {
		return nil, fmt.Errorf("failed to issue token: %w", err)
	}

	return &AuthResponse{
		Token: token,
		User: types.User{
			ID:             account.ID,
			Email:          account.Email,
			Name:           account.Name,
			AvatarURL:      account.AvatarURL,
			GitHubUsername: account.GitHubUsername,
			CreatedAt:      account.CreatedAt,
			UpdatedAt:      account.UpdatedAt,
		},
		Organization: defaultOrg,
	}, nil
}

// GetMe retrieves the profile of the authenticated user
func (s *Service) GetMe(ctx context.Context, userID string) (*types.User, error) {
	account, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &types.User{
		ID:             account.ID,
		Email:          account.Email,
		Name:           account.Name,
		AvatarURL:      account.AvatarURL,
		GitHubUsername: account.GitHubUsername,
		CreatedAt:      account.CreatedAt,
		UpdatedAt:      account.UpdatedAt,
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
