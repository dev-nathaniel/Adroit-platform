package authservice

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler handles HTTP requests for auth endpoints
type Handler struct {
	service *Service
}

// NewHandler creates a new auth HTTP handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers auth endpoints onto a Chi router
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	r.Post("/register", h.HandleRegister)
	r.Post("/login", h.HandleLogin)

	// GitHub OAuth endpoints
	r.Route("/github", func(gr chi.Router) {
		gr.Get("/", h.HandleGitHubLogin)
		gr.Get("/url", h.HandleGitHubAuthURL)
		gr.Get("/callback", h.HandleGitHubCallback)
		gr.Post("/", h.HandleGitHubExchange)
	})

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(middleware.RequireAuth(jwtSecret))
		pr.Get("/me", h.HandleGetMe)
		pr.Post("/password", h.HandleSetPassword)
	})
}

// HandleRegister handles user registration requests
func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	authRes, err := h.service.Register(r.Context(), req)
	if err != nil {
		switch err {
		case ErrInvalidEmail, ErrPasswordTooWeak:
			response.BadRequest(w, err.Error(), nil)
		case ErrUserAlreadyExists:
			response.Conflict(w, err.Error(), nil)
		default:
			response.InternalServerError(w, "Failed to register user")
		}
		return
	}

	response.JSON(w, http.StatusCreated, authRes)
}

// HandleLogin handles user authentication requests
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	authRes, err := h.service.Login(r.Context(), req)
	if err != nil {
		if err == ErrInvalidCredentials {
			response.Unauthorized(w, "Invalid email or password")
			return
		}
		response.InternalServerError(w, "Failed to authenticate")
		return
	}

	response.JSON(w, http.StatusOK, authRes)
}

// HandleGitHubLogin redirects browser directly to GitHub's OAuth authorization page
func (h *Handler) HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = generateRandomState()
	}

	authURLRes, err := h.service.GetGitHubAuthURL(state)
	if err != nil {
		if errors.Is(err, ErrGitHubNotConfigured) {
			response.Error(w, http.StatusNotImplemented, "GITHUB_NOT_CONFIGURED", "GitHub OAuth is not configured on this server", nil)
			return
		}
		response.InternalServerError(w, "Failed to initialize GitHub OAuth")
		return
	}

	// Set state cookie to protect against CSRF attacks
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, authURLRes.URL, http.StatusTemporaryRedirect)
}

// HandleGitHubAuthURL returns the GitHub OAuth URL as JSON without redirecting
func (h *Handler) HandleGitHubAuthURL(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = generateRandomState()
	}

	authURLRes, err := h.service.GetGitHubAuthURL(state)
	if err != nil {
		if errors.Is(err, ErrGitHubNotConfigured) {
			response.Error(w, http.StatusNotImplemented, "GITHUB_NOT_CONFIGURED", "GitHub OAuth is not configured on this server", nil)
			return
		}
		response.InternalServerError(w, "Failed to generate GitHub OAuth URL")
		return
	}

	response.JSON(w, http.StatusOK, authURLRes)
}

// HandleGitHubCallback processes the redirect back from GitHub with authorization code
func (h *Handler) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if ghErr := r.URL.Query().Get("error"); ghErr != "" {
		errDesc := r.URL.Query().Get("error_description")
		response.BadRequest(w, fmt.Sprintf("GitHub OAuth denied: %s (%s)", ghErr, errDesc), nil)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		response.BadRequest(w, "Missing authorization code from GitHub callback", nil)
		return
	}

	// Validate state cookie if present
	state := r.URL.Query().Get("state")
	if cookie, err := r.Cookie("oauth_state"); err == nil && cookie != nil && cookie.Value != "" {
		if state != cookie.Value {
			response.BadRequest(w, "Invalid OAuth state parameter", nil)
			return
		}
		// Clear cookie after successful validation
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_state",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})
	}

	authRes, err := h.service.AuthenticateWithGitHub(r.Context(), code)
	if err != nil {
		if errors.Is(err, ErrGitHubNotConfigured) {
			response.Error(w, http.StatusNotImplemented, "GITHUB_NOT_CONFIGURED", "GitHub OAuth is not configured on this server", nil)
			return
		}
		response.BadRequest(w, fmt.Sprintf("GitHub authentication failed: %v", err), nil)
		return
	}

	// If a frontend redirect URL is configured and client is a browser (does not specifically ask for JSON)
	frontendURL := h.service.GetFrontendURL()
	acceptHeader := r.Header.Get("Accept")
	if frontendURL != "" && !strings.Contains(acceptHeader, "application/json") {
		separator := "?"
		if strings.Contains(frontendURL, "?") {
			separator = "&"
		}
		target := fmt.Sprintf("%s%stoken=%s", frontendURL, separator, url.QueryEscape(authRes.Token))
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}

	response.JSON(w, http.StatusOK, authRes)
}

// HandleGitHubExchange handles direct JSON code exchange for SPAs, mobile, and CLI tools
func (h *Handler) HandleGitHubExchange(w http.ResponseWriter, r *http.Request) {
	var req GitHubAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	if req.Code == "" {
		response.BadRequest(w, "Authorization code is required", nil)
		return
	}

	authRes, err := h.service.AuthenticateWithGitHub(r.Context(), req.Code)
	if err != nil {
		if errors.Is(err, ErrGitHubNotConfigured) {
			response.Error(w, http.StatusNotImplemented, "GITHUB_NOT_CONFIGURED", "GitHub OAuth is not configured on this server", nil)
			return
		}
		response.BadRequest(w, fmt.Sprintf("GitHub authentication failed: %v", err), nil)
		return
	}

	response.JSON(w, http.StatusOK, authRes)
}

// HandleGetMe returns the authenticated user profile
func (h *Handler) HandleGetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "")
		return
	}

	user, err := h.service.GetMe(r.Context(), userID)
	if err != nil {
		if err == ErrUserNotFound {
			response.NotFound(w, "User profile not found")
			return
		}
		response.InternalServerError(w, "Failed to fetch profile")
		return
	}

	response.JSON(w, http.StatusOK, user)
}

// HandleSetPassword handles setting or changing user password
func (h *Handler) HandleSetPassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "")
		return
	}

	var req SetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	if err := h.service.SetPassword(r.Context(), userID, req); err != nil {
		switch err {
		case ErrPasswordTooWeak:
			response.BadRequest(w, err.Error(), nil)
		case ErrInvalidCredentials:
			response.Unauthorized(w, "Current password is incorrect")
		case ErrUserNotFound:
			response.NotFound(w, "User not found")
		default:
			response.InternalServerError(w, "Failed to update password")
		}
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message": "Password updated successfully",
	})
}

func generateRandomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
