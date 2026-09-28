package authservice

import (
	"encoding/json"
	"net/http"

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

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(middleware.RequireAuth(jwtSecret))
		pr.Get("/me", h.HandleGetMe)
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
