package authservice

import (
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// RegisterRequest holds registration inputs
type RegisterRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// LoginRequest holds login credentials
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse returns token and user payload upon successful auth
type AuthResponse struct {
	Token        string              `json:"token"`
	User         types.User          `json:"user"`
	Organization *types.Organization `json:"organization,omitempty"`
}

// UserAccount internal representation with credentials and OAuth metadata
type UserAccount struct {
	ID                string
	Email             string
	Name              string
	PasswordHash      string
	AvatarURL         string
	GitHubID          string
	GitHubUsername    string
	GitHubAccessToken string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// GitHubAuthRequest holds inputs for direct OAuth token exchange
type GitHubAuthRequest struct {
	Code  string `json:"code"`
	State string `json:"state,omitempty"`
}

// GitHubAuthURLResponse provides the GitHub OAuth authorization URL
type GitHubAuthURLResponse struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

