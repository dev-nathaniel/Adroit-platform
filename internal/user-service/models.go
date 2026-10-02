package userservice

import (
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// UpdateProfileRequest holds profile update fields
type UpdateProfileRequest struct {
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// AvatarUploadResponse contains the uploaded avatar URL and user info
type AvatarUploadResponse struct {
	AvatarURL string      `json:"avatar_url"`
	User      *types.User `json:"user,omitempty"`
}

// CreateOrgRequest holds parameters for creating a new workspace
type CreateOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

// AddMemberRequest holds member invitation details
type AddMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"` // owner, admin, member
}

// OrgMemberInfo combines user details with their organization role
type OrgMemberInfo struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

// OrgDetail contains organization info plus member count
type OrgDetail struct {
	types.Organization
	Role        string `json:"user_role"`
	MemberCount int    `json:"member_count"`
}

// CreateInvitationRequest holds inputs for inviting someone to an organization
type CreateInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"` // admin, member
}

// InvitationResponse is returned when an invitation is created, including the raw token and URL
type InvitationResponse struct {
	types.OrganizationInvitation
	Token     string `json:"token"`
	InviteURL string `json:"invite_url"`
}

// InvitationPreview provides public info when an invitee visits the invite link
type InvitationPreview struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	Email            string    `json:"email"`
	Role             string    `json:"role"`
	InvitedByName    string    `json:"invited_by_name"`
	ExpiresAt        time.Time `json:"expires_at"`
	Token            string    `json:"token,omitempty"`
	Status           string    `json:"status,omitempty"`
	UserExists       bool      `json:"user_exists"`
}

// AcceptInvitationResponse is returned when an invitation is successfully accepted
type AcceptInvitationResponse struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	Role             string `json:"role"`
	Message          string `json:"message"`
}
