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
