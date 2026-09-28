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

// UserAccount internal representation with password hash
type UserAccount struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	AvatarURL    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
