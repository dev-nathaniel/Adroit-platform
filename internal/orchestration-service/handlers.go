package orchestrationservice

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// Handler exposes the REST HTTP API for the Orchestration Service
type Handler struct {
	svc *Service
}

// NewHandler creates an initialized Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers HTTP routes with Chi router
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret string) {
	// Public health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]any{
			"service": "orchestration-service",
			"status":  "healthy",
		})
	})

	// Protected routes
	r.Group(func(protected chi.Router) {
		if jwtSecret != "" {
			protected.Use(middleware.RequireAuth(jwtSecret))
		}

		// Service-level deployment & scaling routes
		protected.Post("/services/{id}/deploy", h.handleTriggerDeploy)
		protected.Get("/services/{id}/deployments", h.handleListDeployments)
		protected.Post("/services/{id}/scale", h.handleScaleService)

		// Deployment-specific lifecycle routes
		protected.Get("/deployments/{id}", h.handleGetDeployment)
		protected.Get("/deployments/{id}/spec", h.handleGetDeploymentSpec)
		protected.Get("/deployments/{id}/allocations", h.handleGetAllocations)
		protected.Get("/deployments/{id}/events", h.handleGetDeploymentEvents)
		protected.Post("/deployments/{id}/promote", h.handlePromoteCanary)
		protected.Post("/deployments/{id}/rollback", h.handleRollback)
		protected.Post("/deployments/{id}/cancel", h.handleCancelDeployment)
	})
}

// handleTriggerDeploy initiates a new manual deployment for a service
func (h *Handler) handleTriggerDeploy(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required in URL path", nil)
		return
	}

	var req TriggerDeployRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.BadRequest(w, "Invalid JSON request payload: "+err.Error(), nil)
			return
		}
	}
	req.ServiceID = serviceID

	deployment, err := h.svc.TriggerDeploy(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.NotFound(w, "Service not found: "+serviceID)
			return
		}
		if errors.Is(err, ErrNoSucceededBuild) {
			response.BadRequest(w, "Cannot deploy service: no successful build artifact exists yet", nil)
			return
		}
		if errors.Is(err, ErrBuildNotFound) {
			response.NotFound(w, "Specified build not found")
			return
		}
		response.InternalServerError(w, "Failed to initiate deployment: "+err.Error())
		return
	}

	response.JSON(w, http.StatusAccepted, deployment)
}

// handleListDeployments returns paginated deployment history for a service
func (h *Handler) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}

	offset := (page - 1) * perPage

	items, total, err := h.svc.ListDeployments(r.Context(), serviceID, perPage, offset)
	if err != nil {
		response.InternalServerError(w, "Failed to query deployments: "+err.Error())
		return
	}

	totalPages := (total + perPage - 1) / perPage
	meta := &response.Pagination{
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}

	response.JSONWithMeta(w, http.StatusOK, items, meta)
}

// handleScaleService updates the replica count for a service on Nomad
func (h *Handler) handleScaleService(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if serviceID == "" {
		response.BadRequest(w, "Service ID is required", nil)
		return
	}

	var req ScaleServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body: "+err.Error(), nil)
		return
	}

	if req.Replicas <= 0 {
		response.BadRequest(w, "Replicas count must be at least 1", nil)
		return
	}

	var scaleErr error
	if req.MinReplicas != nil || req.MaxReplicas != nil || req.AutoScalingEnabled != nil {
		scaleErr = h.svc.ScaleServiceWithOptions(r.Context(), serviceID, req)
	} else {
		scaleErr = h.svc.ScaleService(r.Context(), serviceID, req.Replicas)
	}

	if scaleErr != nil {
		if errors.Is(scaleErr, ErrServiceNotFound) {
			response.NotFound(w, "Service not found: "+serviceID)
			return
		}
		response.InternalServerError(w, "Failed to scale service: "+scaleErr.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"service_id": serviceID,
		"replicas":   req.Replicas,
		"status":     "scaling_applied",
	})
}

// handleGetDeployment returns comprehensive deployment details, allocations, and timeline events
func (h *Handler) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	detail, err := h.svc.GetDeployment(r.Context(), deployID)
	if err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			response.NotFound(w, "Deployment not found: "+deployID)
			return
		}
		response.InternalServerError(w, "Failed to fetch deployment details: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, detail)
}

// handleGetDeploymentSpec returns the rendered Nomad HCL job specification (.nomad.hcl)
func (h *Handler) handleGetDeploymentSpec(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	spec, err := h.svc.GetDeploymentSpec(r.Context(), deployID)
	if err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			response.NotFound(w, "Deployment not found: "+deployID)
			return
		}
		response.InternalServerError(w, "Failed to get deployment job spec: "+err.Error())
		return
	}

	// Check if client accepts plain text / hcl
	if r.Header.Get("Accept") == "text/plain" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(spec))
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"deployment_id": deployID,
		"job_spec":      spec,
	})
}

// handleGetAllocations returns the scheduled Nomad allocations for a deployment
func (h *Handler) handleGetAllocations(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	allocs, err := h.svc.GetDeploymentAllocations(r.Context(), deployID)
	if err != nil {
		response.InternalServerError(w, "Failed to get allocations: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, allocs)
}

// handleGetDeploymentEvents returns the rollout audit timeline for a deployment
func (h *Handler) handleGetDeploymentEvents(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	events, err := h.svc.GetDeploymentEvents(r.Context(), deployID)
	if err != nil {
		response.InternalServerError(w, "Failed to get deployment timeline events: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, events)
}

// handlePromoteCanary manually promotes a canary deployment
func (h *Handler) handlePromoteCanary(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	if err := h.svc.PromoteCanary(r.Context(), deployID); err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			response.NotFound(w, "Deployment not found: "+deployID)
			return
		}
		if errors.Is(err, ErrDeploymentNotCanary) {
			response.BadRequest(w, "Deployment is not configured for canary rollout", nil)
			return
		}
		response.InternalServerError(w, "Failed to promote canary deployment: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"deployment_id": deployID,
		"promoted":      true,
		"message":       "Canary successfully promoted. Remaining replicas are provisioning.",
	})
}

// handleRollback initiates a rollback to a specified or previous deployment
func (h *Handler) handleRollback(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	deployment, err := h.svc.Rollback(r.Context(), deployID)
	if err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			response.NotFound(w, "Target deployment not found for rollback: "+deployID)
			return
		}
		response.InternalServerError(w, "Failed to initiate rollback: "+err.Error())
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]any{
		"rollback_to":        deployID,
		"new_deployment_id":  deployment.ID,
		"status":             "rollback_initiated",
	})
}

// handleCancelDeployment aborts a pending or active deployment rollout
func (h *Handler) handleCancelDeployment(w http.ResponseWriter, r *http.Request) {
	deployID := chi.URLParam(r, "id")
	if deployID == "" {
		response.BadRequest(w, "Deployment ID is required", nil)
		return
	}

	if err := h.svc.CancelDeployment(r.Context(), deployID); err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			response.NotFound(w, "Deployment not found: "+deployID)
			return
		}
		if errors.Is(err, ErrCannotCancelDone) {
			response.BadRequest(w, "Cannot cancel a deployment that is already active or failed", nil)
			return
		}
		response.InternalServerError(w, "Failed to cancel deployment: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"deployment_id": deployID,
		"status":        "cancelled",
	})
}
