package gitintegrationservice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// Handler handles HTTP requests for webhooks and Git integrations
type Handler struct {
	service *Service
}

// NewHandler creates a new git-integration-service HTTP handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers all webhook and Git integration endpoints
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	// 1. Ingress Webhook Routes (Public, authenticated via HMAC signature or token)
	r.Route("/webhooks", func(wr chi.Router) {
		wr.Post("/github", h.HandleGitHubWebhook)
		wr.Post("/gitlab", h.HandleGitLabWebhook)
	})

	// 2. Protected Git Provider & Repository Routes
	r.Route("/git", func(gr chi.Router) {
		// Also mount webhooks under /git/webhooks for routing flexibility
		gr.Post("/webhooks/github", h.HandleGitHubWebhook)
		gr.Post("/webhooks/gitlab", h.HandleGitLabWebhook)

		// Authenticated management endpoints
		gr.Group(func(auth chi.Router) {
			auth.Use(middleware.RequireAuth(jwtSecret))

			auth.Get("/providers", h.HandleListProviders)
			auth.Post("/providers/connect", h.HandleConnectProvider)
			auth.Delete("/providers/{provider}", h.HandleDisconnectProvider)

			auth.Get("/{provider}/repos", h.HandleListRepos)
			auth.Get("/{provider}/repos/{owner}/{repo}/branches", h.HandleListBranches)

			auth.Post("/services/{id}/webhook", h.HandleSetupServiceWebhook)
			auth.Get("/services/{id}/webhook", h.HandleGetServiceWebhookInfo)

			auth.Get("/webhooks/events", h.HandleListWebhookEvents)
		})
	})
}

// ============================================================================
// Webhook Handlers
// ============================================================================

// HandleGitHubWebhook processes incoming GitHub webhook events
func (h *Handler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	eventType := r.Header.Get("X-GitHub-Event")
	signature := r.Header.Get("X-Hub-Signature-256")

	if eventType == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_HEADER", "X-GitHub-Event header is required", nil)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "READ_ERROR", "Failed to read request body", nil)
		return
	}

	res, err := h.service.ProcessGitHubWebhook(r.Context(), deliveryID, eventType, signature, body)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid X-Hub-Signature-256 signature", nil)
			return
		}
		response.Error(w, http.StatusBadRequest, "WEBHOOK_ERROR", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// HandleGitLabWebhook processes incoming GitLab webhook events
func (h *Handler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	deliveryID := r.Header.Get("X-Gitlab-Delivery")
	eventType := r.Header.Get("X-Gitlab-Event")
	token := r.Header.Get("X-Gitlab-Token")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "READ_ERROR", "Failed to read request body", nil)
		return
	}

	res, err := h.service.ProcessGitLabWebhook(r.Context(), deliveryID, eventType, token, body)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid X-Gitlab-Token secret token", nil)
			return
		}
		response.Error(w, http.StatusBadRequest, "WEBHOOK_ERROR", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// ============================================================================
// Provider Account Handlers
// ============================================================================

// HandleListProviders lists all connected VCS providers for the authenticated user
func (h *Handler) HandleListProviders(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	providers, err := h.service.ListProviders(r.Context(), userID)
	if err != nil {
		response.InternalServerError(w, "Failed to list providers")
		return
	}

	response.JSON(w, http.StatusOK, providers)
}

// HandleConnectProvider links a personal access token or OAuth credential
func (h *Handler) HandleConnectProvider(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	var req ConnectProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request payload", nil)
		return
	}

	req.Provider = types.GitProviderType(strings.ToLower(string(req.Provider)))
	if req.Provider != types.GitProviderGitHub && req.Provider != types.GitProviderGitLab {
		response.Error(w, http.StatusBadRequest, "INVALID_PROVIDER", "Provider must be 'github' or 'gitlab'", nil)
		return
	}

	conn, err := h.service.ConnectProvider(r.Context(), userID, req)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "CONNECT_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, conn)
}

// HandleDisconnectProvider unlinks a provider account
func (h *Handler) HandleDisconnectProvider(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	provider := types.GitProviderType(strings.ToLower(chi.URLParam(r, "provider")))
	if provider != types.GitProviderGitHub && provider != types.GitProviderGitLab {
		response.Error(w, http.StatusBadRequest, "INVALID_PROVIDER", "Provider must be 'github' or 'gitlab'", nil)
		return
	}

	if err := h.service.DisconnectProvider(r.Context(), userID, provider); err != nil {
		response.InternalServerError(w, "Failed to disconnect provider")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message":  "Provider disconnected successfully",
		"provider": string(provider),
	})
}

// ============================================================================
// Repository & Branch Discovery Handlers
// ============================================================================

// HandleListRepos lists repositories from the user's connected provider account
func (h *Handler) HandleListRepos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	provider := types.GitProviderType(strings.ToLower(chi.URLParam(r, "provider")))
	if provider != types.GitProviderGitHub && provider != types.GitProviderGitLab {
		response.Error(w, http.StatusBadRequest, "INVALID_PROVIDER", "Provider must be 'github' or 'gitlab'", nil)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	repos, err := h.service.ListRepositories(r.Context(), userID, provider, page, perPage)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "FETCH_REPOS_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, repos)
}

// HandleListBranches lists branches for a specific repository
func (h *Handler) HandleListBranches(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Authentication required")
		return
	}

	provider := types.GitProviderType(strings.ToLower(chi.URLParam(r, "provider")))
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	if owner == "" || repo == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_PARAMS", "owner and repo parameters are required", nil)
		return
	}

	branches, err := h.service.ListBranches(r.Context(), userID, provider, owner, repo)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "FETCH_BRANCHES_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, branches)
}

// ============================================================================
// Service Webhook Setup Handlers
// ============================================================================

// HandleSetupServiceWebhook configures or auto-registers a webhook for a service
func (h *Handler) HandleSetupServiceWebhook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")

	if serviceID == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_ID", "service id is required", nil)
		return
	}

	res, err := h.service.SetupServiceWebhook(r.Context(), userID, serviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		response.Error(w, http.StatusBadRequest, "SETUP_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// HandleGetServiceWebhookInfo gets manual webhook instructions and details
func (h *Handler) HandleGetServiceWebhookInfo(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_ID", "service id is required", nil)
		return
	}

	info, err := h.service.GetServiceWebhookInfo(r.Context(), serviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFound(w, "Service not found")
			return
		}
		response.Error(w, http.StatusBadRequest, "INFO_FAILED", err.Error(), nil)
		return
	}

	response.JSON(w, http.StatusOK, info)
}

// HandleListWebhookEvents returns recent webhook delivery audit events
func (h *Handler) HandleListWebhookEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	events, err := h.service.ListRecentWebhookEvents(r.Context(), limit)
	if err != nil {
		response.InternalServerError(w, "Failed to list webhook events")
		return
	}

	response.JSON(w, http.StatusOK, events)
}
