package buildservice

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler handles HTTP requests for build lifecycle operations
type Handler struct {
	service *Service
}

// NewHandler creates a new build-service HTTP handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers all build routes onto Chi routers
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	r.Group(func(auth chi.Router) {
		auth.Use(middleware.RequireAuth(jwtSecret))

		// Builds root routes
		auth.Route("/builds", func(br chi.Router) {
			br.Get("/", h.HandleListAllBuilds)
			br.Post("/", h.HandleTriggerBuildDirect)
			br.Get("/{id}", h.HandleGetBuild)
			br.Post("/{id}/cancel", h.HandleCancelBuild)
			br.Get("/{id}/logs", h.HandleGetBuildLogs)
		})

		// Service-nested routes: /services/{id}/builds
		auth.Route("/services/{id}/builds", func(sr chi.Router) {
			sr.Post("/", h.HandleTriggerBuild)
			sr.Get("/", h.HandleListServiceBuilds)
		})
	})
}

// ============================================================================
// HANDLERS
// ============================================================================

// HandleTriggerBuild triggers a build for the specified service ID in the URL param
func (h *Handler) HandleTriggerBuild(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")
	if strings.TrimSpace(serviceID) == "" {
		response.BadRequest(w, "service ID is required in URL", nil)
		return
	}

	var req TriggerBuildRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.BadRequest(w, "Invalid JSON body", nil)
			return
		}
	}

	build, err := h.service.TriggerBuild(r.Context(), userID, serviceID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, build)
}

// HandleTriggerBuildDirect triggers a build via POST /builds with service_id in JSON body
func (h *Handler) HandleTriggerBuildDirect(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	type directReq struct {
		ServiceID string `json:"service_id"`
		TriggerBuildRequest
	}

	var req directReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body", nil)
		return
	}

	if strings.TrimSpace(req.ServiceID) == "" {
		response.BadRequest(w, "service_id is required", nil)
		return
	}

	build, err := h.service.TriggerBuild(r.Context(), userID, req.ServiceID, req.TriggerBuildRequest)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, build)
}

// HandleListServiceBuilds lists build history for a service
func (h *Handler) HandleListServiceBuilds(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	serviceID := chi.URLParam(r, "id")
	if strings.TrimSpace(serviceID) == "" {
		response.BadRequest(w, "service ID is required in URL", nil)
		return
	}

	limit := parseQueryInt(r, "limit", 20)
	offset := parseQueryInt(r, "offset", 0)
	status := r.URL.Query().Get("status")

	resp, err := h.service.ListBuilds(r.Context(), userID, serviceID, limit, offset, status)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, resp)
}

// HandleListAllBuilds allows querying builds with query param ?service_id=
func (h *Handler) HandleListAllBuilds(w http.ResponseWriter, r *http.Request) {
	serviceID := r.URL.Query().Get("service_id")
	if serviceID != "" {
		chi.RouteContext(r.Context()).URLParams.Add("id", serviceID)
		h.HandleListServiceBuilds(w, r)
		return
	}

	response.BadRequest(w, "query parameter 'service_id' is required", nil)
}

// HandleGetBuild retrieves build details by ID
func (h *Handler) HandleGetBuild(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	buildID := chi.URLParam(r, "id")
	if strings.TrimSpace(buildID) == "" {
		response.BadRequest(w, "build ID is required", nil)
		return
	}

	build, err := h.service.GetBuild(r.Context(), userID, buildID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, build)
}

// HandleCancelBuild aborts an in-flight or queued build
func (h *Handler) HandleCancelBuild(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	buildID := chi.URLParam(r, "id")
	if strings.TrimSpace(buildID) == "" {
		response.BadRequest(w, "build ID is required", nil)
		return
	}

	build, err := h.service.CancelBuild(r.Context(), userID, buildID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, build)
}

// HandleGetBuildLogs retrieves build log lines
func (h *Handler) HandleGetBuildLogs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	buildID := chi.URLParam(r, "id")
	if strings.TrimSpace(buildID) == "" {
		response.BadRequest(w, "build ID is required", nil)
		return
	}

	limit := parseQueryInt(r, "limit", 1000)
	offset := parseQueryInt(r, "offset", 0)

	logs, err := h.service.GetBuildLogs(r.Context(), userID, buildID, limit, offset)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, logs)
}

// ============================================================================
// HELPERS
// ============================================================================

func parseQueryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrForbidden):
		response.Forbidden(w, err.Error())
	case errors.Is(err, ErrUnauthorized):
		response.Unauthorized(w, err.Error())
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrBuildFinished):
		response.BadRequest(w, err.Error(), nil)
	case errors.Is(err, ErrConflict), errors.Is(err, ErrBuildActive):
		response.Conflict(w, err.Error(), nil)
	default:
		response.InternalServerError(w, err.Error())
	}
}
