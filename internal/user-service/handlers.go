package userservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler handles HTTP requests for user and organization endpoints
type Handler struct {
	service *Service
}

// NewHandler creates a new user HTTP handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers endpoints onto a Chi router
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	// Public Routes (no authentication required)
	r.Get("/invitations/{token}", h.HandleGetInvitationPreview)
	r.Get("/avatars/{filename}", h.HandleGetAvatar)

	// Protected Routes (requires valid JWT token)
	r.Group(func(auth chi.Router) {
		auth.Use(middleware.RequireAuth(jwtSecret))

		// User Profile
		auth.Get("/profile", h.HandleGetProfile)
		auth.Patch("/profile", h.HandleUpdateProfile)
		auth.Post("/profile/avatar", h.HandleUploadAvatar)
		auth.Delete("/profile/avatar", h.HandleDeleteAvatar)

		// Organizations
		auth.Get("/organizations", h.HandleListOrganizations)
		auth.Post("/organizations", h.HandleCreateOrganization)
		auth.Get("/organizations/{orgID}", h.HandleGetOrganization)
		auth.Post("/organizations/{orgID}/members", h.HandleAddMember)
		auth.Get("/organizations/{orgID}/members", h.HandleListMembers)

		// Invitations
		auth.Post("/organizations/{orgID}/invitations", h.HandleCreateInvitation)
		auth.Get("/organizations/{orgID}/invitations", h.HandleListInvitations)
		auth.Delete("/organizations/{orgID}/invitations/{inviteID}", h.HandleRevokeInvitation)
		auth.Post("/invitations/{token}/accept", h.HandleAcceptInvitation)
	})
}

// HandleGetProfile returns the caller's profile
func (h *Handler) HandleGetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	profile, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		if err == ErrNotFound {
			response.NotFound(w, "User profile not found")
			return
		}
		response.InternalServerError(w, "Failed to retrieve profile")
		return
	}
	response.JSON(w, http.StatusOK, profile)
}

// HandleUpdateProfile updates caller profile
func (h *Handler) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid request body", nil)
		return
	}

	updated, err := h.service.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		response.InternalServerError(w, "Failed to update profile")
		return
	}
	response.JSON(w, http.StatusOK, updated)
}

// HandleUploadAvatar handles multipart/form-data upload of user profile picture
func (h *Handler) HandleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	// 1. Enforce 2MB maximum payload size
	const maxUploadSize = 2 << 20 // 2MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		response.BadRequest(w, "Uploaded file exceeds 2MB limit or invalid form data", nil)
		return
	}

	// 2. Locate avatar file in multipart form (check "avatar" then "file")
	file, fileHeader, err := r.FormFile("avatar")
	if err != nil {
		file, fileHeader, err = r.FormFile("file")
		if err != nil {
			response.BadRequest(w, "Avatar image file is required under field 'avatar' or 'file'", nil)
			return
		}
	}
	defer file.Close()

	// 3. Inspect magic bytes for content-type detection
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		response.BadRequest(w, "Failed to inspect uploaded file", nil)
		return
	}

	contentType := http.DetectContentType(buf[:n])
	var ext string
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	default:
		response.BadRequest(w, "Unsupported file format. Only JPEG, PNG, WebP, and GIF images are allowed", map[string]string{
			"detected_type": contentType,
			"filename":      fileHeader.Filename,
		})
		return
	}

	// Reconstruct the file stream from the read magic bytes + rest of file
	stream := io.MultiReader(bytes.NewReader(buf[:n]), file)

	// 4. Generate unique, collision-resistant filename
	cleanUserID := strings.ReplaceAll(userID, "/", "_")
	randomSuffix := uuid.New().String()[:8]
	filename := fmt.Sprintf("avatar_%s_%d_%s%s", cleanUserID, time.Now().Unix(), randomSuffix, ext)

	// 5. Save to storage & update user record
	updatedUser, avatarURL, err := h.service.UploadAvatar(r.Context(), userID, filename, stream)
	if err != nil {
		response.InternalServerError(w, "Failed to upload avatar")
		return
	}

	response.JSON(w, http.StatusOK, AvatarUploadResponse{
		AvatarURL: avatarURL,
		User:      updatedUser,
	})
}

// HandleDeleteAvatar clears the caller's avatar URL
func (h *Handler) HandleDeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	updatedUser, err := h.service.RemoveAvatar(r.Context(), userID)
	if err != nil {
		response.InternalServerError(w, "Failed to remove avatar")
		return
	}

	response.JSON(w, http.StatusOK, updatedUser)
}

// HandleGetAvatar serves a stored avatar image by filename
func (h *Handler) HandleGetAvatar(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	cleanName := filepath.Base(filepath.Clean(filename))
	if cleanName == "." || cleanName == "/" || cleanName == "" {
		response.BadRequest(w, "Invalid filename", nil)
		return
	}

	reader, size, err := h.service.Storage().Open(cleanName)
	if err != nil {
		if os.IsNotExist(err) {
			response.NotFound(w, "Avatar not found")
			return
		}
		response.InternalServerError(w, "Failed to retrieve avatar")
		return
	}
	defer reader.Close()

	buf := make([]byte, 512)
	n, _ := reader.Read(buf)
	contentType := http.DetectContentType(buf[:n])

	w.Header().Set("Content-Type", contentType)
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf[:n])
	_, _ = io.Copy(w, reader)
}

// HandleListOrganizations lists all workspaces for the caller
func (h *Handler) HandleListOrganizations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	orgs, err := h.service.ListOrganizations(r.Context(), userID)
	if err != nil {
		response.InternalServerError(w, "Failed to list organizations")
		return
	}
	response.JSON(w, http.StatusOK, orgs)
}

// HandleCreateOrganization creates a new workspace
func (h *Handler) HandleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid request body", nil)
		return
	}

	org, err := h.service.CreateOrganization(r.Context(), userID, req)
	if err != nil {
		if err == ErrOrgNameRequired {
			response.BadRequest(w, err.Error(), nil)
			return
		}
		response.InternalServerError(w, "Failed to create organization")
		return
	}
	response.JSON(w, http.StatusCreated, org)
}

// HandleGetOrganization retrieves single organization details
func (h *Handler) HandleGetOrganization(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	orgID := chi.URLParam(r, "orgID")

	org, role, err := h.service.GetOrganization(r.Context(), userID, orgID)
	if err != nil {
		if err == ErrUnauthorized {
			response.Forbidden(w, "You are not a member of this organization")
			return
		}
		if err == ErrNotFound {
			response.NotFound(w, "Organization not found")
			return
		}
		response.InternalServerError(w, "Failed to get organization")
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"organization": org,
		"role":         role,
	})
}

// HandleAddMember adds a teammate to the workspace
func (h *Handler) HandleAddMember(w http.ResponseWriter, r *http.Request) {
	callerID := middleware.GetUserID(r.Context())
	orgID := chi.URLParam(r, "orgID")

	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid request body", nil)
		return
	}

	if err := h.service.AddMember(r.Context(), callerID, orgID, req); err != nil {
		switch err {
		case ErrPermissionDenied:
			response.Forbidden(w, err.Error())
		case ErrInvalidRole:
			response.BadRequest(w, err.Error(), nil)
		case ErrAlreadyMember:
			response.Conflict(w, err.Error(), nil)
		default:
			response.InternalServerError(w, err.Error())
		}
		return
	}

	response.JSON(w, http.StatusCreated, map[string]string{
		"status":  "success",
		"message": "Member added to organization successfully",
	})
}

// HandleListMembers lists all members of the workspace
func (h *Handler) HandleListMembers(w http.ResponseWriter, r *http.Request) {
	callerID := middleware.GetUserID(r.Context())
	orgID := chi.URLParam(r, "orgID")

	members, err := h.service.ListMembers(r.Context(), callerID, orgID)
	if err != nil {
		if err == ErrUnauthorized {
			response.Forbidden(w, "You do not have access to view this organization's members")
			return
		}
		response.InternalServerError(w, "Failed to list members")
		return
	}
	response.JSON(w, http.StatusOK, members)
}

// HandleCreateInvitation creates a pending workspace invitation
func (h *Handler) HandleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	callerID := middleware.GetUserID(r.Context())
	orgID := chi.URLParam(r, "orgID")

	var req CreateInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid request body", nil)
		return
	}

	invitation, err := h.service.CreateInvitation(r.Context(), callerID, orgID, req)
	if err != nil {
		switch err {
		case ErrPermissionDenied:
			response.Forbidden(w, err.Error())
		case ErrInvalidRole, ErrInvalidEmail:
			response.BadRequest(w, err.Error(), nil)
		case ErrAlreadyMember, ErrPendingInvitationExists:
			response.Conflict(w, err.Error(), nil)
		case ErrNotFound:
			response.NotFound(w, "Organization not found")
		default:
			response.InternalServerError(w, err.Error())
		}
		return
	}

	response.JSON(w, http.StatusCreated, invitation)
}

// HandleGetInvitationPreview returns public preview details for an invite token
func (h *Handler) HandleGetInvitationPreview(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	preview, err := h.service.GetInvitationPreview(r.Context(), token)
	if err != nil {
		switch err {
		case ErrInvalidInvitation:
			response.NotFound(w, "Invitation is invalid or has already been used")
		case ErrInvitationExpired:
			response.Error(w, http.StatusGone, "INVITATION_EXPIRED", "This invitation link has expired", nil)
		default:
			response.InternalServerError(w, "Failed to retrieve invitation details")
		}
		return
	}

	response.JSON(w, http.StatusOK, preview)
}

// HandleAcceptInvitation allows authenticated users to accept an invitation
func (h *Handler) HandleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	token := chi.URLParam(r, "token")

	res, err := h.service.AcceptInvitation(r.Context(), userID, token)
	if err != nil {
		switch err {
		case ErrInvalidInvitation:
			response.NotFound(w, "Invitation is invalid or has already been used")
		case ErrInvitationExpired:
			response.Error(w, http.StatusGone, "INVITATION_EXPIRED", "This invitation link has expired", nil)
		case ErrInvitationEmailMismatch:
			response.Forbidden(w, "Your account email does not match the invitation email")
		default:
			response.InternalServerError(w, err.Error())
		}
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// HandleListInvitations lists pending invitations for an organization
func (h *Handler) HandleListInvitations(w http.ResponseWriter, r *http.Request) {
	callerID := middleware.GetUserID(r.Context())
	orgID := chi.URLParam(r, "orgID")

	invitations, err := h.service.ListInvitations(r.Context(), callerID, orgID)
	if err != nil {
		if err == ErrPermissionDenied {
			response.Forbidden(w, "Only owners and admins can view pending invitations")
			return
		}
		response.InternalServerError(w, "Failed to list invitations")
		return
	}

	response.JSON(w, http.StatusOK, invitations)
}

// HandleRevokeInvitation revokes a pending workspace invitation
func (h *Handler) HandleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	callerID := middleware.GetUserID(r.Context())
	orgID := chi.URLParam(r, "orgID")
	inviteID := chi.URLParam(r, "inviteID")

	if err := h.service.RevokeInvitation(r.Context(), callerID, orgID, inviteID); err != nil {
		switch err {
		case ErrPermissionDenied:
			response.Forbidden(w, "Only owners and admins can revoke invitations")
		case ErrNotFound:
			response.NotFound(w, "Invitation not found")
		default:
			response.InternalServerError(w, "Failed to revoke invitation")
		}
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Invitation revoked successfully",
	})
}

